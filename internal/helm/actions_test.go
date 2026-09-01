package helm

import (
	"context"
	"errors"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type stubRunner struct {
	args      [][]string
	envs      [][]string
	out       string
	err       error
	available error
}

func (s *stubRunner) Run(_ context.Context, args, env []string) (string, error) {
	s.args = append(s.args, args)
	s.envs = append(s.envs, env)
	if s.err != nil {
		return "", s.err
	}
	return s.out, nil
}

func (s *stubRunner) Available() error {
	return s.available
}

func acting(t *testing.T, runner Runner, objs ...*corev1.Secret) *Service {
	t.Helper()
	client := k8sfake.NewClientset()
	for _, obj := range objs {
		_, err := client.CoreV1().Secrets(obj.Namespace).Create(context.Background(), obj, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("seed secret: %v", err)
		}
	}
	return NewService(client, mirrorMeta(client), runner, nil, actionRepositories(), api.ContextRef{Name: "kind-spinoza"})
}

func TestRollbackRunsHelmWithThePinnedContext(t *testing.T) {
	runner := &stubRunner{out: "Rollback was a success! Happy Helming!"}
	service := acting(t, runner, helmSecret(sampleRelease()))

	got, err := service.Rollback(context.Background(), "demo", "podinfo", 2)
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}

	if len(runner.args) != 1 {
		t.Fatalf("helm ran %d times, want once", len(runner.args))
	}
	args := strings.Join(runner.args[0], " ")
	if args != "rollback podinfo 2 --namespace demo --kube-context kind-spinoza" {
		t.Fatalf("args = %q", args)
	}
	if got.Revision != 2 {
		t.Fatalf("revision = %d, want 2", got.Revision)
	}
	if !strings.Contains(got.Message, "Happy Helming") {
		t.Fatalf("message = %q, want helm's own output", got.Message)
	}
}

func TestUninstallRunsHelmWithThePinnedContext(t *testing.T) {
	runner := &stubRunner{}
	service := acting(t, runner, helmSecret(sampleRelease()))

	got, err := service.Uninstall(context.Background(), "demo", "podinfo")
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	args := strings.Join(runner.args[0], " ")
	if args != "uninstall podinfo --namespace demo --kube-context kind-spinoza" {
		t.Fatalf("args = %q", args)
	}
	if got.Message != "uninstalled podinfo from demo" {
		t.Fatalf("message = %q, want a plain summary when helm says nothing", got.Message)
	}
}

func TestActionsTellHelmWhichDriverHoldsTheRelease(t *testing.T) {
	runner := &stubRunner{}
	entry := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sh.helm.release.v1.podinfo.v1",
			Namespace: "demo",
			Labels:    map[string]string{"owner": "helm", "name": "podinfo", "version": "1"},
		},
		Data: map[string]string{releaseKey: "rubbish"},
	}
	client := k8sfake.NewClientset(entry)
	service := NewService(client, mirrorMeta(client), runner, nil, nil, api.ContextRef{Name: "kind-spinoza"})

	_, err := service.Uninstall(context.Background(), "demo", "podinfo")
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	if len(runner.envs[0]) != 1 || runner.envs[0][0] != "HELM_DRIVER=configmap" {
		t.Fatalf("env = %v, want the configmap driver named", runner.envs[0])
	}
}

func TestActionsSayNothingAboutTheDriverForTheDefaultOne(t *testing.T) {
	runner := &stubRunner{}
	service := acting(t, runner, helmSecret(sampleRelease()))

	_, err := service.Uninstall(context.Background(), "demo", "podinfo")
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	if len(runner.envs[0]) != 0 {
		t.Fatalf("env = %v, want nothing added for the default driver", runner.envs[0])
	}
}

func TestActionsRefuseNamesThatCouldBeFlags(t *testing.T) {
	runner := &stubRunner{}
	service := acting(t, runner)

	_, rollbackErr := service.Rollback(context.Background(), "demo", "--kubeconfig=/etc/shadow", 1)
	_, uninstallErr := service.Uninstall(context.Background(), "--all-namespaces", "podinfo")

	if rollbackErr == nil || uninstallErr == nil {
		t.Fatal("a flag-shaped name was accepted")
	}
	if len(runner.args) != 0 {
		t.Fatalf("helm ran %d times for refused input", len(runner.args))
	}
}

func TestARollbackNeedsARevision(t *testing.T) {
	runner := &stubRunner{}
	service := acting(t, runner, helmSecret(sampleRelease()))

	_, err := service.Rollback(context.Background(), "demo", "podinfo", 0)

	if err == nil {
		t.Fatal("a rollback with no revision was accepted")
	}
	if len(runner.args) != 0 {
		t.Fatal("helm ran without a revision to roll back to")
	}
}

func TestActionsRefuseAReleaseThatIsNotThere(t *testing.T) {
	runner := &stubRunner{}
	service := acting(t, runner)

	_, err := service.Uninstall(context.Background(), "demo", "ghost")

	if !errors.Is(err, ErrNoRelease) {
		t.Fatalf("err = %v, want it to say there is no such release", err)
	}
	if len(runner.args) != 0 {
		t.Fatal("helm ran for a release that does not exist")
	}
}

func TestAFailedHelmRunIsReported(t *testing.T) {
	runner := &stubRunner{err: errors.New("release: not found")}
	service := acting(t, runner, helmSecret(sampleRelease()))

	_, err := service.Rollback(context.Background(), "demo", "podinfo", 1)

	if err == nil {
		t.Fatal("a failed helm run reported success")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v, want helm's own message", err)
	}
}

func TestSupportReportsAMissingHelm(t *testing.T) {
	service := acting(t, &stubRunner{available: ErrNoHelmBinary})

	got := service.Support()

	if got.Available {
		t.Fatal("a missing helm was reported as available")
	}
	if !strings.Contains(got.Reason, "not found") {
		t.Fatalf("reason = %q, want it to say helm is missing", got.Reason)
	}
	if got.Binary != DefaultBinary {
		t.Fatalf("binary = %q, want %s", got.Binary, DefaultBinary)
	}
}

func TestSupportReportsAHelmItCanRun(t *testing.T) {
	got := acting(t, &stubRunner{}).Support()

	if !got.Available {
		t.Fatal("a runnable helm was reported as unavailable")
	}
	if got.Reason != "" {
		t.Fatalf("reason = %q, want none", got.Reason)
	}
}

func TestAServiceWithNoRunnerRefusesToAct(t *testing.T) {
	service := NewService(k8sfake.NewClientset(), mirrorMeta(k8sfake.NewClientset()), nil, nil, nil, api.ContextRef{Name: "kind-spinoza"})

	support := service.Support()
	_, err := service.Uninstall(context.Background(), "demo", "podinfo")

	if support.Available {
		t.Fatal("a service with no runner claimed helm works")
	}
	if err == nil {
		t.Fatal("a service with no runner accepted an uninstall")
	}
}

func TestTheRealRunnerReportsAMissingBinary(t *testing.T) {
	runner := NewRunner("definitely-not-a-real-binary-name")

	err := runner.Available()

	if !errors.Is(err, ErrNoHelmBinary) {
		t.Fatalf("err = %v, want the missing-binary error", err)
	}
}

func TestTheRealRunnerCarriesTheCommandOutput(t *testing.T) {
	runner := NewRunner("echo")

	out, err := runner.Run(context.Background(), []string{"hello", "there"}, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if out != "hello there" {
		t.Fatalf("out = %q, want the command's stdout", out)
	}
}

func TestTheRealRunnerCarriesTheFailureMessage(t *testing.T) {
	runner := NewRunner("sh")

	_, err := runner.Run(context.Background(), []string{"-c", "echo boom >&2; exit 1"}, nil)

	if err == nil {
		t.Fatal("a failing command reported success")
	}
	if err.Error() != "boom" {
		t.Fatalf("err = %v, want the stderr line", err)
	}
}

func TestTheRealRunnerFallsBackToTheExitError(t *testing.T) {
	runner := NewRunner("sh")

	_, err := runner.Run(context.Background(), []string{"-c", "exit 3"}, nil)

	if err == nil {
		t.Fatal("a failing command with no stderr reported success")
	}
}

func TestTheRealRunnerPassesTheEnvironmentThrough(t *testing.T) {
	runner := NewRunner("sh")

	out, err := runner.Run(context.Background(), []string{"-c", "echo $HELM_DRIVER"}, []string{"HELM_DRIVER=configmap"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if out != "configmap" {
		t.Fatalf("out = %q, want the driver from the added environment", out)
	}
}

func TestTheDefaultBinaryIsUsedWhenNoneIsNamed(t *testing.T) {
	runner, ok := NewRunner("").(*helmRunner)

	if !ok {
		t.Fatal("NewRunner returned something other than the helm runner")
	}
	if runner.binary != DefaultBinary {
		t.Fatalf("binary = %q, want %s", runner.binary, DefaultBinary)
	}
}

func TestTheRealRunnerAcceptsABinaryOnPath(t *testing.T) {
	if NewRunner("sh").Available() != nil {
		t.Fatal("sh was reported as missing")
	}
}

func TestActionsReportARefusedStorageRead(t *testing.T) {
	client := k8sfake.NewClientset()
	client.PrependReactor("list", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("secrets is forbidden")
	})
	service := NewService(client, mirrorMeta(client), &stubRunner{}, nil, nil, api.ContextRef{Name: "kind-spinoza"})

	_, rollbackErr := service.Rollback(context.Background(), "demo", "podinfo", 1)
	_, uninstallErr := service.Uninstall(context.Background(), "demo", "podinfo")

	if rollbackErr == nil || uninstallErr == nil {
		t.Fatal("a refused storage read reported success")
	}
}

func TestAConfigMapListFailureIsReported(t *testing.T) {
	client := k8sfake.NewClientset()
	meta := mirrorMeta(client)
	meta.PrependReactor("list", "configmaps", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("configmaps is forbidden")
	})
	service := NewService(client, meta, nil, nil, nil, api.ContextRef{Name: "kind-spinoza"})

	_, err := service.List(context.Background())

	if err == nil {
		t.Fatal("a refused configmap list reported success")
	}
}

func TestAConfigMapWithNoReleaseKeyIsSkipped(t *testing.T) {
	entry := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "not-a-release",
			Namespace: "demo",
			Labels:    map[string]string{"owner": "helm"},
		},
		Data: map[string]string{"other": "value"},
	}
	service := NewService(k8sfake.NewClientset(entry), mirrorMeta(k8sfake.NewClientset(entry)), nil, nil, nil, api.ContextRef{Name: "kind-spinoza"})

	got, err := service.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(got.Releases) != 0 {
		t.Fatalf("releases = %v, want none", got.Releases)
	}
}

func TestValuesThatCannotBeMarshalledReadEmpty(t *testing.T) {
	got := valuesOf(payload{Config: map[string]any{"bad": func() {}}})

	if got != "" {
		t.Fatalf("values = %q, want nothing when the config will not marshal", got)
	}
}

func TestAnActionNamesTheKubeconfigTheContextCameFrom(t *testing.T) {
	runner := &stubRunner{}
	client := k8sfake.NewClientset(helmSecret(sampleRelease()))
	service := NewService(client, mirrorMeta(client), runner, nil, nil, api.ContextRef{
		Name:       "kind-spinoza",
		Kubeconfig: "/tmp/spinoza.kubeconfig",
	})

	_, err := service.Uninstall(context.Background(), "demo", "podinfo")
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	args := strings.Join(runner.args[0], " ")
	want := "uninstall podinfo --namespace demo --kube-context kind-spinoza" +
		" --kubeconfig /tmp/spinoza.kubeconfig"
	if args != want {
		t.Fatalf("args = %q, want %q", args, want)
	}
}

func TestAnActionLeavesHelmsOwnLookupAloneWhenNoFileIsNamed(t *testing.T) {
	runner := &stubRunner{}
	service := acting(t, runner, helmSecret(sampleRelease()))

	_, err := service.Uninstall(context.Background(), "demo", "podinfo")
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	if strings.Contains(strings.Join(runner.args[0], " "), "--kubeconfig") {
		t.Fatalf("args = %v, want no file forced on helm", runner.args[0])
	}
}
