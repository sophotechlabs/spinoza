package kubeconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const oneContext = `apiVersion: v1
kind: Config
current-context: alpha
clusters:
- name: c1
  cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
contexts:
- name: alpha
  context:
    cluster: c1
    namespace: alpha-ns
    user: u1
users:
- name: u1
  user:
    token: t
`

const otherContext = `apiVersion: v1
kind: Config
current-context: beta
clusters:
- name: c2
  cluster:
    server: https://127.0.0.2:6443
    insecure-skip-tls-verify: true
contexts:
- name: beta
  context:
    cluster: c2
    user: u2
users:
- name: u2
  user:
    token: t
`

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	err := os.WriteFile(path, []byte(content), 0o600)
	if err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func newSources(t *testing.T, fallback string) *Sources {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "kubeconfigs.json"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return NewSources(fallback, store)
}

func TestTheDefaultKubeconfigIsAlwaysListedFirst(t *testing.T) {
	fallback := writeFile(t, "config", oneContext)
	sources := newSources(t, fallback)

	list := sources.List()

	if len(list) != 1 {
		t.Fatalf("kubeconfigs = %d, want just the default one", len(list))
	}
	if list[0].Path != "" {
		t.Fatalf("path = %q, want the default kubeconfig to carry no path", list[0].Path)
	}
	if list[0].Label != fallback {
		t.Fatalf("label = %q, want the file it reads", list[0].Label)
	}
	if list[0].Removable {
		t.Fatal("the default kubeconfig was offered up for removal")
	}
	if len(list[0].Contexts) != 1 || list[0].Contexts[0].Name != "alpha" {
		t.Fatalf("contexts = %v", list[0].Contexts)
	}
}

func TestAnAddedKubeconfigIsListedOnItsOwn(t *testing.T) {
	sources := newSources(t, writeFile(t, "config", oneContext))
	other := writeFile(t, "other.yaml", otherContext)

	err := sources.Add(other)
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	list := sources.List()
	if len(list) != 2 {
		t.Fatalf("kubeconfigs = %d, want the default and the added one", len(list))
	}
	if list[1].Path != other {
		t.Fatalf("path = %q, want the file spinoza will read", list[1].Path)
	}
	if !list[1].Removable {
		t.Fatal("an added kubeconfig cannot be removed again")
	}
	if len(list[1].Contexts) != 1 || list[1].Contexts[0].Name != "beta" {
		t.Fatalf("contexts = %v; an added kubeconfig is read on its own, never merged", list[1].Contexts)
	}
	if len(list[0].Contexts) != 1 || list[0].Contexts[0].Name != "alpha" {
		t.Fatalf("default contexts = %v; adding a file must not change what the default reads", list[0].Contexts)
	}
}

func TestListReadsEveryKubeconfigAgainEachTime(t *testing.T) {
	path := writeFile(t, "config", oneContext)
	sources := newSources(t, path)
	if len(sources.List()[0].Contexts) != 1 {
		t.Fatal("the first read found no contexts")
	}

	err := os.WriteFile(path, []byte(otherContext), 0o600)
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}

	contexts := sources.List()[0].Contexts
	if len(contexts) != 1 || contexts[0].Name != "beta" {
		t.Fatalf("contexts = %v, want the file as it is on disk now", contexts)
	}
}

func TestAKubeconfigThatCannotBeReadIsListedWithItsReason(t *testing.T) {
	sources := newSources(t, writeFile(t, "config", oneContext))
	broken := writeFile(t, "broken.yaml", oneContext)
	if err := sources.Add(broken); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := os.Remove(broken); err != nil {
		t.Fatalf("remove: %v", err)
	}

	list := sources.List()

	if list[1].Error == "" {
		t.Fatal("a kubeconfig that has since gone missing was listed as if it were fine")
	}
	if len(list[1].Contexts) != 0 {
		t.Fatalf("contexts = %v, want none", list[1].Contexts)
	}
	if list[0].Error != "" {
		t.Fatalf("default error = %q; one unreadable file must not take the others down", list[0].Error)
	}
}

func TestAKubeconfigWithNoContextsSaysSo(t *testing.T) {
	sources := newSources(t, writeFile(t, "config", "apiVersion: v1\nkind: Config\n"))

	list := sources.List()

	if list[0].Error != noContexts {
		t.Fatalf("error = %q, want %q", list[0].Error, noContexts)
	}
}

func TestAddingRefusesAFileThatIsNotAKubeconfig(t *testing.T) {
	sources := newSources(t, writeFile(t, "config", oneContext))

	err := sources.Add(writeFile(t, "notes.txt", "not: [a kubeconfig"))

	if err == nil {
		t.Fatal("a file that does not parse was remembered as a kubeconfig")
	}
	if len(sources.List()) != 1 {
		t.Fatalf("kubeconfigs = %d, want the bad one left off", len(sources.List()))
	}
}

func TestAddingRefusesAKubeconfigWithNoContexts(t *testing.T) {
	sources := newSources(t, writeFile(t, "config", oneContext))

	err := sources.Add(writeFile(t, "empty.yaml", "apiVersion: v1\nkind: Config\n"))

	if err == nil {
		t.Fatal("a kubeconfig with nothing to connect to was remembered")
	}
}

func TestAddingRefusesAFileThatIsNotThere(t *testing.T) {
	sources := newSources(t, writeFile(t, "config", oneContext))

	err := sources.Add(filepath.Join(t.TempDir(), "absent.yaml"))

	if err == nil {
		t.Fatal("a kubeconfig that does not exist was remembered")
	}
}

func TestAddingRefusesTheKubeconfigAlreadyReadByDefault(t *testing.T) {
	fallback := writeFile(t, "config", oneContext)
	sources := newSources(t, fallback)

	err := sources.Add(fallback)

	if err == nil {
		t.Fatal("the default kubeconfig was added a second time")
	}
	if !strings.Contains(err.Error(), "already reads") {
		t.Fatalf("error = %v, want it to name the reason", err)
	}
}

func TestAddingRefusesAnEmptyPath(t *testing.T) {
	sources := newSources(t, writeFile(t, "config", oneContext))

	err := sources.Add("   ")

	if err == nil {
		t.Fatal("an empty path was remembered")
	}
}

func TestRemovingDropsTheKubeconfig(t *testing.T) {
	sources := newSources(t, writeFile(t, "config", oneContext))
	other := writeFile(t, "other.yaml", otherContext)
	if err := sources.Add(other); err != nil {
		t.Fatalf("add: %v", err)
	}

	err := sources.Remove(other)
	if err != nil {
		t.Fatalf("remove: %v", err)
	}

	if len(sources.List()) != 1 {
		t.Fatalf("kubeconfigs = %d, want only the default left", len(sources.List()))
	}
}

func TestRemovingRejectsAnEmptyPath(t *testing.T) {
	sources := newSources(t, writeFile(t, "config", oneContext))

	err := sources.Remove("")

	if err == nil {
		t.Fatal("an empty path was accepted as something to remove")
	}
}

func TestARelativePathIsRememberedAsAnAbsoluteOne(t *testing.T) {
	sources := newSources(t, "")
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte(oneContext), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Chdir(dir)

	err := sources.Add("config")
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	if !filepath.IsAbs(sources.List()[1].Path) {
		t.Fatalf("path = %q, want an absolute one; the working directory is not spinoza's to rely on", sources.List()[1].Path)
	}
}

func TestATildePathIsRememberedFromTheHomeDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.WriteFile(filepath.Join(home, "cluster.yaml"), []byte(oneContext), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	sources := newSources(t, "")

	err := sources.Add("~/cluster.yaml")
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	if sources.List()[1].Path != filepath.Join(home, "cluster.yaml") {
		t.Fatalf("path = %q, want the tilde expanded", sources.List()[1].Path)
	}
}

func TestResolveLeavesAnAbsolutePathAlone(t *testing.T) {
	sources := newSources(t, "")

	resolved, err := sources.Resolve("/tmp/one.yaml")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if resolved != filepath.Clean("/tmp/one.yaml") {
		t.Fatalf("resolved = %q", resolved)
	}
}

func TestResolveReportsAHomeItCannotFind(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")

	_, err := Resolve("~/cluster.yaml")

	if err == nil {
		t.Fatal("a tilde path resolved without a home directory")
	}
}

func TestAFallbackThatCannotBeResolvedIsKeptAsGiven(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")

	sources := newSources(t, "~/cluster.yaml")

	if sources.resolved != "~/cluster.yaml" {
		t.Fatalf("resolved = %q, want the flag kept verbatim when it cannot be resolved", sources.resolved)
	}
}
