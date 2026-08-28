package helm

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func storedIn(driver string) *Service {
	client := k8sfake.NewClientset()
	labels := map[string]string{"owner": "helm", "name": "podinfo", versionLabel: "1"}
	meta := metav1.ObjectMeta{Namespace: "demo", Name: "sh.helm.release.v1.podinfo.v1", Labels: labels}
	if driver == DriverConfigMap {
		entry := &corev1.ConfigMap{ObjectMeta: meta, Data: map[string]string{releaseKey: "body"}}
		client = k8sfake.NewClientset(entry)
	}
	if driver == DriverSecret {
		secret := &corev1.Secret{
			ObjectMeta: meta,
			Type:       storageType,
			Data:       map[string][]byte{releaseKey: []byte("body")},
		}
		client = k8sfake.NewClientset(secret)
	}
	return NewService(client, mirrorMeta(client), nil, nil, nil, api.ContextRef{Name: "kind-spinoza"})
}

func TestTheDefaultDriverIsSecrets(t *testing.T) {
	t.Setenv(driverEnv, "")

	if DefaultDriver() != DriverSecret {
		t.Fatalf("driver = %q, want %q", DefaultDriver(), DriverSecret)
	}
}

func TestTheEnvironmentCanAskForConfigMaps(t *testing.T) {
	t.Setenv(driverEnv, DriverConfigMap)

	if DefaultDriver() != DriverConfigMap {
		t.Fatalf("driver = %q, want %q", DefaultDriver(), DriverConfigMap)
	}
}

func TestAnEnvironmentAskingForSomethingElseFallsBackToSecrets(t *testing.T) {
	t.Setenv(driverEnv, "sql")

	if DefaultDriver() != DriverSecret {
		t.Fatalf("driver = %q, want %q", DefaultDriver(), DriverSecret)
	}
}

func TestAReleaseIsAnsweredForByWhereItIsActuallyKept(t *testing.T) {
	t.Setenv(driverEnv, "")
	service := storedIn(DriverConfigMap)

	got := service.ReleaseDriver(context.Background(), "demo", "podinfo")

	if got != DriverConfigMap {
		t.Fatalf("driver = %q, want where the release is", got)
	}
}

func TestAReleaseKeptInSecretsIsAnsweredForAsOne(t *testing.T) {
	t.Setenv(driverEnv, DriverConfigMap)
	service := storedIn(DriverSecret)

	got := service.ReleaseDriver(context.Background(), "demo", "podinfo")

	if got != DriverSecret {
		t.Fatalf("driver = %q, want where the release is", got)
	}
}

func TestAReleaseThatIsNotThereIsAnsweredForAsANewOne(t *testing.T) {
	t.Setenv(driverEnv, DriverConfigMap)
	service := storedIn(DriverSecret)

	got := service.ReleaseDriver(context.Background(), "demo", "nothing-here")

	if got != DriverConfigMap {
		t.Fatalf("driver = %q, want where a new release would go", got)
	}
}

func TestAnEmptyNameIsAnsweredForAsANewRelease(t *testing.T) {
	t.Setenv(driverEnv, "")
	service := storedIn(DriverConfigMap)

	got := service.ReleaseDriver(context.Background(), "demo", "")

	if got != DriverSecret {
		t.Fatalf("driver = %q, want the default for a release with no name", got)
	}
}

func TestANameThatIsNotAKubernetesNameIsNotAsked(t *testing.T) {
	t.Setenv(driverEnv, "")
	service := storedIn(DriverConfigMap)

	got := service.ReleaseDriver(context.Background(), "demo", "Not A Name")

	if got != DriverSecret {
		t.Fatalf("driver = %q, want the default", got)
	}
}

func TestANamespaceThatIsNotAKubernetesNameIsNotAsked(t *testing.T) {
	t.Setenv(driverEnv, "")
	service := storedIn(DriverConfigMap)

	got := service.ReleaseDriver(context.Background(), "Not A Namespace", "podinfo")

	if got != DriverSecret {
		t.Fatalf("driver = %q, want the default", got)
	}
}

func TestAServiceWithNoClientAnswersWithTheDefault(t *testing.T) {
	t.Setenv(driverEnv, DriverConfigMap)
	service := NewService(nil, nil, nil, nil, nil, api.ContextRef{})

	got := service.ReleaseDriver(context.Background(), "demo", "podinfo")

	if got != DriverConfigMap {
		t.Fatalf("driver = %q, want the default", got)
	}
}

func TestNoHelmServiceAnswersWithTheDefault(t *testing.T) {
	t.Setenv(driverEnv, "")
	var service *Service

	got := service.ReleaseDriver(context.Background(), "demo", "podinfo")

	if got != DriverSecret {
		t.Fatalf("driver = %q, want the default", got)
	}
}
