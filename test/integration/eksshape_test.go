//go:build integration

package integration

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/cluster"
	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/kube"
)

const eksArn = "arn:aws:eks:us-east-1:123456789012:cluster/niio-prod"

const eksAlias = "niio-prod"

func kindCluster(t *testing.T) *clientcmdapi.Cluster {
	t.Helper()
	raw, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
	if err != nil {
		t.Fatalf("load kubeconfig: %v", err)
	}
	name := os.Getenv("SPINOZA_TEST_CONTEXT")
	held, ok := raw.Contexts[name]
	if !ok {
		t.Fatalf("context %q is not in the default kubeconfig", name)
	}
	found, ok := raw.Clusters[held.Cluster]
	if !ok {
		t.Fatalf("cluster %q is not in the default kubeconfig", held.Cluster)
	}
	if len(found.CertificateAuthorityData) == 0 {
		t.Fatalf("cluster %q carries no inline certificate authority, so it cannot stand in for EKS", held.Cluster)
	}
	return found
}

func serviceAccountToken(t *testing.T, loaded *kube.Bundle) string {
	t.Helper()
	ctx := t.Context()
	_, err := loaded.Clientset.CoreV1().ServiceAccounts(namespace).Create(ctx, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "eks-shape"},
	}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create service account: %v", err)
	}
	expires := int64((2 * time.Hour).Seconds())
	issued, err := loaded.Clientset.CoreV1().ServiceAccounts(namespace).CreateToken(ctx, "eks-shape", &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{ExpirationSeconds: &expires},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("issue a token: %v", err)
	}
	return issued.Status.Token
}

func credentialPluginShim(t *testing.T, token string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "aws")
	script := "#!/bin/sh\nprintf '%s' '" +
		`{"apiVersion":"client.authentication.k8s.io/v1beta1","kind":"ExecCredential","status":{"token":"` + token + `"}}` +
		"'\n"
	err := os.WriteFile(path, []byte(script), 0o600)
	if err != nil {
		t.Fatalf("write the plugin shim: %v", err)
	}
	chmodErr := os.Chmod(path, 0o700)
	if chmodErr != nil {
		t.Fatalf("make the plugin shim runnable: %v", chmodErr)
	}
	return path
}

func eksShapedKubeconfig(t *testing.T, contextName, server string, authority []byte, shim string) string {
	t.Helper()
	config := clientcmdapi.NewConfig()
	config.Clusters[eksArn] = &clientcmdapi.Cluster{Server: server, CertificateAuthorityData: authority}
	config.AuthInfos[eksArn] = &clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{
		APIVersion: "client.authentication.k8s.io/v1beta1",
		Command:    shim,
		Args:       []string{"--region", "us-east-1", "eks", "get-token", "--cluster-name", "niio-prod", "--output", "json"},
	}}
	config.Contexts[contextName] = &clientcmdapi.Context{Cluster: eksArn, AuthInfo: eksArn}
	config.CurrentContext = contextName
	path := filepath.Join(t.TempDir(), "eks-kubeconfig")
	err := clientcmd.WriteToFile(*config, path)
	if err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	return path
}

func someOtherAuthority(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate a key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "some other cluster"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create a certificate: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestAnEksShapedKubeconfigOpensTheCluster(t *testing.T) {
	loaded := bundle(t)
	found := kindCluster(t)
	shim := credentialPluginShim(t, serviceAccountToken(t, loaded))

	for _, contextName := range []string{eksAlias, eksArn} {
		path := eksShapedKubeconfig(t, contextName, found.Server, found.CertificateAuthorityData, shim)

		opened, err := kube.LoadContext(api.ContextRef{Name: contextName, Kubeconfig: path}, kube.Options{})
		if err != nil {
			t.Fatalf("load %q: %v", contextName, err)
		}
		_, descs, discErr := discovery.List(opened.Discovery)
		if discErr != nil {
			t.Fatalf("discover through %q: %v", contextName, discErr)
		}
		if len(descs) == 0 {
			t.Fatalf("context %q listed no resource types", contextName)
		}
	}
}

func TestAKubeconfigWhoseAuthorityTheClusterDoesNotPresentNamesTheFile(t *testing.T) {
	loaded := bundle(t)
	found := kindCluster(t)
	shim := credentialPluginShim(t, serviceAccountToken(t, loaded))
	path := eksShapedKubeconfig(t, eksAlias, found.Server, someOtherAuthority(t), shim)

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	_, err := cluster.New(ctx, cluster.Options{Kubeconfig: path, Context: eksAlias, SyncTimeout: 30 * time.Second})

	want := `context "niio-prod" in ` + path + ` carries a certificate authority the cluster did not present ` +
		`(x509: certificate signed by unknown authority). Check that this is the kubeconfig kubectl reads, then recreate the entry`
	if err == nil {
		t.Fatal("the cluster opened with a certificate authority it does not use")
	}
	if err.Error() != want {
		t.Fatalf("error = %q\nwant    %q", err.Error(), want)
	}
	if strings.Contains(err.Error(), "lists no resource types") {
		t.Fatalf("error = %q, want the trust problem named rather than the generic discovery failure", err.Error())
	}
}
