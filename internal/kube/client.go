package kube

import (
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"time"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/reach"
)

type Bundle struct {
	Config    *restclient.Config
	Clientset *kubernetes.Clientset
	Dynamic   dynamic.Interface
	Discovery discovery.CachedDiscoveryInterface
	Mapper    *restmapper.DeferredDiscoveryRESTMapper
	Reach     *reach.Sink
	Warnings  *WarningSink
	Ref       api.ContextRef
	Namespace string
}

type Options struct {
	Kubeconfig string
	QPS        float32
	Burst      int
}

func (o Options) orDefaults() Options {
	if o.QPS == 0 {
		o.QPS = clientQPS
	}
	if o.Burst == 0 {
		o.Burst = clientBurst
	}
	return o
}

func rulesFor(path string) *clientcmd.ClientConfigLoadingRules {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if path == "" {
		return rules
	}
	rules.ExplicitPath = path
	return rules
}

func configFor(ref api.ContextRef, fallback string) clientcmd.ClientConfig {
	path := ref.Kubeconfig
	if path == "" {
		path = fallback
	}
	overrides := &clientcmd.ConfigOverrides{CurrentContext: ref.Name}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rulesFor(path), overrides)
}

func Read(path string) ([]api.KubeContext, error) {
	raw, err := configFor(api.ContextRef{Kubeconfig: path}, "").RawConfig()
	if err != nil {
		return nil, fmt.Errorf("kubeconfig: %w", err)
	}
	out := make([]api.KubeContext, 0, len(raw.Contexts))
	for _, name := range slices.Sorted(maps.Keys(raw.Contexts)) {
		entry := raw.Contexts[name]
		out = append(out, api.KubeContext{
			Cluster:   entry.Cluster,
			Name:      name,
			Namespace: entry.Namespace,
		})
	}
	return out, nil
}

func Label(path string) string {
	if path != "" {
		return path
	}
	return strings.Join(clientcmd.NewDefaultClientConfigLoadingRules().Precedence, ", ")
}

const (
	clientQPS        = 50
	clientBurst      = 100
	discoveryTimeout = 30 * time.Second
)

func boundedDiscovery(restConfig *restclient.Config) (discovery.DiscoveryInterface, error) {
	timed := restclient.CopyConfig(restConfig)
	timed.Timeout = discoveryTimeout
	client, err := discovery.NewDiscoveryClientForConfig(timed)
	if err != nil {
		return nil, fmt.Errorf("kube discovery: %w", err)
	}
	return client, nil
}

func LoadContext(ref api.ContextRef, options Options) (*Bundle, error) {
	options = options.orDefaults()
	clientConfig := configFor(ref, options.Kubeconfig)

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("kube client config: %w", err)
	}
	restConfig.QPS = options.QPS
	restConfig.Burst = options.Burst
	warnings := newWarningLogger(slog.Default())
	restConfig.WarningHandler = warnings
	answers := reach.New()
	restConfig.Wrap(answers.Wrap)

	cs, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("kube clientset: %w", err)
	}

	dyn, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("kube dynamic: %w", err)
	}

	bounded, err := boundedDiscovery(restConfig)
	if err != nil {
		return nil, err
	}
	cached := memory.NewMemCacheClient(bounded)
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(cached)

	resolved := ref
	if resolved.Kubeconfig == "" {
		resolved.Kubeconfig = options.Kubeconfig
	}
	if resolved.Name == "" {
		rawConfig, rawErr := clientConfig.RawConfig()
		if rawErr == nil {
			resolved.Name = rawConfig.CurrentContext
		}
	}

	namespace := ""
	ns, _, nsErr := clientConfig.Namespace()
	if nsErr == nil {
		namespace = ns
	}

	return &Bundle{
		Config:    restConfig,
		Clientset: cs,
		Dynamic:   dyn,
		Discovery: cached,
		Mapper:    mapper,
		Reach:     answers,
		Warnings:  warnings,
		Ref:       resolved,
		Namespace: namespace,
	}, nil
}
