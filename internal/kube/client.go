package kube

import (
	"fmt"
	"slices"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

type Bundle struct {
	Config    *restclient.Config
	Clientset *kubernetes.Clientset
	Dynamic   dynamic.Interface
	Discovery discovery.CachedDiscoveryInterface
	Mapper    *restmapper.DeferredDiscoveryRESTMapper
	Context   string
	Namespace string
}

func Load() (*Bundle, error) {
	return LoadContext("")
}

func configFor(contextName string) clientcmd.ClientConfig {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{CurrentContext: contextName}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
}

func Contexts() ([]string, string, error) {
	raw, err := configFor("").RawConfig()
	if err != nil {
		return nil, "", fmt.Errorf("kubeconfig: %w", err)
	}
	names := make([]string, 0, len(raw.Contexts))
	for name := range raw.Contexts {
		names = append(names, name)
	}
	slices.Sort(names)
	return names, raw.CurrentContext, nil
}

const (
	clientQPS   = 50
	clientBurst = 100
)

func LoadContext(contextName string) (*Bundle, error) {
	clientConfig := configFor(contextName)

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("kube client config: %w", err)
	}
	restConfig.QPS = clientQPS
	restConfig.Burst = clientBurst

	cs, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("kube clientset: %w", err)
	}

	dyn, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("kube dynamic: %w", err)
	}

	cached := memory.NewMemCacheClient(cs.Discovery())
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(cached)

	resolved := contextName
	if resolved == "" {
		rawConfig, rawErr := clientConfig.RawConfig()
		if rawErr == nil {
			resolved = rawConfig.CurrentContext
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
		Context:   resolved,
		Namespace: namespace,
	}, nil
}
