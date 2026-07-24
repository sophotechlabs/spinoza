package kube

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func Load() (*kubernetes.Clientset, string, string, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{}
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, "", "", fmt.Errorf("kube client config: %w", err)
	}

	cs, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, "", "", fmt.Errorf("kube clientset: %w", err)
	}

	contextName := ""
	rawConfig, err := clientConfig.RawConfig()
	if err == nil {
		contextName = rawConfig.CurrentContext
	}

	namespace := ""
	ns, _, err := clientConfig.Namespace()
	if err == nil {
		namespace = ns
	}

	return cs, contextName, namespace, nil
}
