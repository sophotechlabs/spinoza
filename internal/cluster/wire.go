package cluster

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/metadata"

	"github.com/sophotechlabs/spinoza/internal/access"
	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/charts"
	"github.com/sophotechlabs/spinoza/internal/compare"
	"github.com/sophotechlabs/spinoza/internal/debugcontainer"
	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/exec"
	"github.com/sophotechlabs/spinoza/internal/helm"
	"github.com/sophotechlabs/spinoza/internal/jsonschema"
	"github.com/sophotechlabs/spinoza/internal/kube"
	"github.com/sophotechlabs/spinoza/internal/kubeconfig"
	"github.com/sophotechlabs/spinoza/internal/nodeshell"
	"github.com/sophotechlabs/spinoza/internal/portforward"
	"github.com/sophotechlabs/spinoza/internal/prom"
	"github.com/sophotechlabs/spinoza/internal/protect"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

func New(ctx context.Context, options Options) (*Cluster, error) {
	promTarget, targetErr := prom.ParseTarget(options.PromSpec)
	if targetErr != nil {
		return nil, targetErr
	}
	sources := kubeconfig.NewSources(options.Kubeconfig, openStore())
	cluster := newCluster(ctx, func(buildCtx context.Context, ref api.ContextRef) (*connection, error) {
		manager, bundle, err := build(buildCtx, ref, options, promTarget)
		if err != nil {
			return nil, err
		}
		return &connection{manager: manager, ref: bundle.Ref, host: bundle.Config.Host}, nil
	}, sources, openProtection(), options.OpenTimeout)
	cluster.useReader(readerFor(options))
	cluster.useLister(listerFor(options))
	if options.Context != "" {
		useErr := cluster.Use(api.ContextRef{Name: options.Context})
		if useErr != nil {
			return cluster, useErr
		}
	}
	return cluster, nil
}

const readAcrossTimeout = 15 * time.Second

func objectsIn(dyn dynamic.Interface, gvr schema.GroupVersionResource, namespace string) dynamic.ResourceInterface {
	if namespace == "" {
		return dyn.Resource(gvr)
	}
	return dyn.Resource(gvr).Namespace(namespace)
}

func readerFor(options Options) reader {
	return func(ctx context.Context, ref api.ContextRef, target api.ObjectRef) (string, error) {
		bundle, err := kube.LoadContext(ref, kube.Options{
			Kubeconfig: options.Kubeconfig,
			QPS:        options.ClientQPS,
			Burst:      options.ClientBurst,
		})
		if err != nil {
			return "", fmt.Errorf("kube: %w", err)
		}
		gvr := schema.GroupVersionResource{Group: target.Group, Version: target.Version, Resource: target.Resource}
		read, cancel := context.WithTimeout(ctx, readAcrossTimeout)
		defer cancel()
		found, getErr := objectsIn(bundle.Dynamic, gvr, target.Namespace).Get(read, target.Name, metav1.GetOptions{})
		if getErr != nil {
			return "", getErr
		}
		return compare.YAML(found)
	}
}

const listAcrossTimeout = 60 * time.Second

func listerFor(options Options) lister {
	return func(ctx context.Context, ref api.ContextRef, target api.ObjectRef) ([]*unstructured.Unstructured, error) {
		bundle, err := kube.LoadContext(ref, kube.Options{
			Kubeconfig: options.Kubeconfig,
			QPS:        options.ClientQPS,
			Burst:      options.ClientBurst,
		})
		if err != nil {
			return nil, fmt.Errorf("kube: %w", err)
		}
		gvr := schema.GroupVersionResource{Group: target.Group, Version: target.Version, Resource: target.Resource}
		list, cancel := context.WithTimeout(ctx, listAcrossTimeout)
		defer cancel()
		found, listErr := objectsIn(bundle.Dynamic, gvr, target.Namespace).List(list, metav1.ListOptions{})
		if listErr != nil {
			return nil, listErr
		}
		out := make([]*unstructured.Unstructured, 0, len(found.Items))
		for i := range found.Items {
			out = append(out, &found.Items[i])
		}
		return out, nil
	}
}

func openProtection() *protect.Store {
	path, pathErr := protect.DefaultPath()
	if pathErr != nil {
		slog.Warn("protected clusters will not be remembered", "error", pathErr)
	}
	store, openErr := protect.Open(path)
	if openErr != nil {
		slog.Warn("the protected cluster list could not be read", "error", openErr)
	}
	return store
}

func openStore() *kubeconfig.Store {
	path, pathErr := kubeconfig.DefaultPath()
	if pathErr != nil {
		slog.Warn("kubeconfigs you add will not be remembered", "error", pathErr)
	}
	store, openErr := kubeconfig.Open(path)
	if openErr != nil {
		slog.Warn("the remembered kubeconfig list could not be read", "error", openErr)
	}
	return store
}

func unreachable(name string, discErr error) error {
	if discErr == nil {
		return fmt.Errorf("context %q lists no resource types", name)
	}
	plugin := credentialPlugin(discErr)
	if plugin != "" {
		slog.Warn("a credential plugin failed", "context", name, "plugin", plugin, "error", discErr)
		return fmt.Errorf("context %q could not get credentials: %s failed. Check that it runs in your shell", name, plugin)
	}
	return fmt.Errorf("context %q lists no resource types: %w", name, discErr)
}

func build(ctx context.Context, ref api.ContextRef, options Options, promTarget prom.Target) (*resources.Manager, *kube.Bundle, error) {
	bundle, err := kube.LoadContext(ref, kube.Options{
		Kubeconfig: options.Kubeconfig,
		QPS:        options.ClientQPS,
		Burst:      options.ClientBurst,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("kube: %w", err)
	}
	cats, descs, discErr := discovery.List(bundle.Discovery)
	if len(descs) == 0 {
		return nil, nil, unreachable(bundle.Ref.Name, discErr)
	}
	if discErr != nil {
		slog.Warn("discovery came back incomplete", "error", discErr)
	}
	slog.Info("connected to a cluster", "context", bundle.Ref.Name, "resourceTypes", len(descs), "categories", len(cats))
	perms := access.New(bundle.Clientset)
	schemas := jsonschema.NewClient(bundle.Discovery.OpenAPIV3)
	forwards := portforward.NewRegistry(
		ctx,
		portforward.NewRunner(bundle.Clientset, bundle.Config),
		portforward.NewResolver(bundle.Clientset),
		portforward.NewProber(bundle.Clientset),
	)
	shells := exec.NewService(
		exec.NewStreamer(bundle.Clientset, bundle.Config),
		exec.NewImages(bundle.Clientset),
	)
	debugger := debugcontainer.NewService(
		debugcontainer.NewRunner(options.KubectlBinary),
		bundle.Clientset,
		options.DebugImage,
		bundle.Ref,
		perms,
	)
	index := charts.New(ctx, &http.Client{Timeout: 30 * time.Second}, charts.DefaultTTL)
	meta := metaClient(bundle)
	releases := helm.NewService(
		bundle.Clientset,
		meta,
		helm.NewRunner(options.HelmBinary),
		index,
		helm.Repositories(helm.RepositoryConfig()),
		bundle.Ref,
	)
	nodeShells := nodeshell.NewService(
		bundle.Clientset,
		options.NodeShellImage,
		options.NodeShellNS,
		options.NodeShell,
		perms,
	)
	promClient := prom.NewClient(bundle.Clientset, promTarget)
	mgr := resources.NewManager(ctx, resources.Deps{
		Limits: resources.Limits{
			SyncTimeout:     options.SyncTimeout,
			WarmConcurrency: options.WarmConcurrency,
			Counts: resources.CountLimits{
				Budget:      options.CountBudget,
				PerType:     options.CountPerType,
				Concurrency: options.CountConcurrency,
			},
		},
		Dynamic:     bundle.Dynamic,
		Metadata:    meta,
		Clientset:   bundle.Clientset,
		Schemas:     schemas,
		Forwards:    forwards,
		Shells:      shells,
		Debugger:    debugger,
		NodeShells:  nodeShells,
		Helm:        releases,
		Charts:      index,
		Prometheus:  promClient,
		Perms:       perms,
		Reach:       bundle.Reach,
		Warnings:    bundle.Warnings,
		Categories:  cats,
		Descriptors: descs,
		Columns:     options.Columns,
	})
	mgr.UseDiscovery(bundle.Discovery, discErr)
	return mgr, bundle, nil
}

func metaClient(bundle *kube.Bundle) metadata.Interface {
	client, err := metadata.NewForConfig(bundle.Config)
	if err != nil {
		slog.Warn("searching by name is unavailable", "error", err)
		return nil
	}
	return client
}
