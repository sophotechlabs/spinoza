package kube

import (
	"fmt"

	"k8s.io/client-go/discovery"
	memory "k8s.io/client-go/discovery/cached/memory"
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
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{}
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("kube client config: %w", err)
	}

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

	contextName := ""
	rawConfig, rawErr := clientConfig.RawConfig()
	if rawErr == nil {
		contextName = rawConfig.CurrentContext
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
		Context:   contextName,
		Namespace: namespace,
	}, nil
}
