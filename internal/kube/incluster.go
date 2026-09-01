package kube

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	inClusterName = "in-cluster"
	//nolint:gosec // the path kubernetes mounts the token at, not a token
	tokenPath      = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	caPath         = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	kubeconfigDir  = "spinoza"
	kubeconfigFile = "in-cluster.kubeconfig"
)

func InCluster() bool {
	_, err := restclient.InClusterConfig()
	return !errors.Is(err, restclient.ErrNotInCluster)
}

func WriteInClusterKubeconfig(dir string) (string, error) {
	if dir == "" {
		return "", errors.New("no directory to write the in-cluster kubeconfig into")
	}
	config, err := restclient.InClusterConfig()
	if err != nil {
		return "", fmt.Errorf("in-cluster config: %w", err)
	}
	return writeToolKubeconfig(dir, config.Host)
}

func writeToolKubeconfig(dir, host string) (string, error) {
	doc := clientcmdapi.Config{
		APIVersion:     "v1",
		Kind:           "Config",
		CurrentContext: inClusterName,
		Clusters: map[string]*clientcmdapi.Cluster{
			inClusterName: {
				Server:               host,
				CertificateAuthority: caPath,
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			inClusterName: {TokenFile: tokenPath},
		},
		Contexts: map[string]*clientcmdapi.Context{
			inClusterName: {Cluster: inClusterName, AuthInfo: inClusterName},
		},
	}
	body, marshalErr := clientcmd.Write(doc)
	if marshalErr != nil {
		return "", fmt.Errorf("in-cluster kubeconfig: %w", marshalErr)
	}
	if mkErr := os.MkdirAll(filepath.Join(dir, kubeconfigDir), 0o700); mkErr != nil {
		return "", fmt.Errorf("in-cluster kubeconfig: %w", mkErr)
	}
	path := filepath.Join(dir, kubeconfigDir, kubeconfigFile)
	if writeErr := os.WriteFile(path, body, 0o600); writeErr != nil {
		return "", fmt.Errorf("in-cluster kubeconfig: %w", writeErr)
	}
	return path, nil
}
