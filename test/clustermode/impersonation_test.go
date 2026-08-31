//go:build clustermode

package clustermode

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func scaleTo(namespace, name string, replicas int) string {
	query := url.Values{
		"action":    {"scale"},
		"group":     {"apps"},
		"version":   {"v1"},
		"resource":  {"deployments"},
		"namespace": {namespace},
		"name":      {name},
		"replicas":  {strconv.Itoa(replicas)},
	}
	return "/api/action?" + query.Encode()
}

func logsFor(namespace, pod string) api.ClientMsg {
	return api.ClientMsg{
		Type:      "logs-subscribe",
		Namespace: namespace,
		Name:      pod,
		TailLines: 5,
	}
}

func TestEveryWayOfReachingTheClusterActsAsThePersonWhoAsked(t *testing.T) {
	values := oidcValues()
	values["nodeShell"] = "true"
	values["auth.adminGroups[1]"] = "platform"
	values["extraArgs[0]"] = "--pprof"
	deploy(t, values)

	paymentsPod := podIn(t, "payments")
	defaultPod := podIn(t, "default")

	t.Run("the service account itself may not do any of this", func(t *testing.T) {
		account := "system:serviceaccount:spinoza:spinoza"
		for _, verb := range []string{"create pods/exec", "create pods/portforward", "create pods"} {
			parts := strings.SplitN(verb, " ", 2)
			out, err := maybeKubectl(t, "auth", "can-i", parts[1], "-n", "payments", "--as", account)
			if err == nil && strings.TrimSpace(out) == "yes" {
				t.Fatalf("the service account may %s, so nothing below proves impersonation", verb)
			}
		}
	})

	t.Run("a write", func(t *testing.T) {
		bob := signIn(t, "bob")
		status, message := post(t, bob, scaleTo("payments", "web", 2))
		if status != http.StatusOK {
			t.Fatalf("scaling in its own namespace gave %d: %s", status, message)
		}
		status, message = post(t, bob, scaleTo("default", "other", 2))
		if status != http.StatusForbidden {
			t.Fatalf("scaling elsewhere gave %d: %s", status, message)
		}
		if !strings.Contains(messageOf(t, message), `User "bob"`) {
			t.Fatalf("the cluster refused %q, want it to name bob", message)
		}
	})

	t.Run("a shell", func(t *testing.T) {
		opened := shell(t, signIn(t, "alice"), "/api/exec?namespace=payments&pod="+paymentsPod)
		if !strings.HasPrefix(opened, "OPENED") {
			t.Fatalf("alice could not open a shell: %s", opened)
		}
		refused := shell(t, signIn(t, "bob"), "/api/exec?namespace=default&pod="+defaultPod)
		if !strings.Contains(refused, `User "bob"`) {
			t.Fatalf("the cluster refused %q, want it to name bob", refused)
		}
	})

	t.Run("a port forward, which would land on the server rather than on anyone", func(t *testing.T) {
		status, message := post(t, signIn(t, "alice"),
			"/api/portforward?kind=Pod&namespace=payments&name="+paymentsPod+"&port=80")
		if status != http.StatusForbidden {
			t.Fatalf("status = %d, want %d even for an admin", status, http.StatusForbidden)
		}
		if !strings.Contains(messageOf(t, message), "run it yourself") {
			t.Fatalf("body = %q, want it to say why", message)
		}
	})

	t.Run("a profile, which carries whatever the caches hold", func(t *testing.T) {
		refused, message := read(t, signIn(t, "carol"), "/debug/pprof/symbol")
		if refused != http.StatusForbidden {
			t.Fatalf("a viewer got %d from a profile: %s", refused, truncate(message))
		}
		if !strings.Contains(messageOf(t, message), "this needs admin") {
			t.Fatalf("body = %q, want it to name the role", message)
		}
		allowed, _ := read(t, signIn(t, "alice"), "/debug/pprof/symbol")
		if allowed != http.StatusOK {
			t.Fatalf("an admin got %d from a profile", allowed)
		}
	})

	t.Run("a debug container", func(t *testing.T) {
		bob := signIn(t, "bob")
		status, message := post(t, bob, "/api/debug?namespace=default&pod="+defaultPod)
		if status < 400 {
			t.Fatalf("bob started a debug container where he only reads: %d %s", status, message)
		}
		if !strings.Contains(messageOf(t, message), "bob") {
			t.Fatalf("kubectl refused %q, want it to name bob", message)
		}
	})

	t.Run("a node shell", func(t *testing.T) {
		refused := shell(t, signIn(t, "bob"), "/api/nodeshell?node="+aNode(t))
		if strings.HasPrefix(refused, "OPENED") {
			t.Fatal("bob opened a root shell on a node with no binding for it")
		}
		if !strings.Contains(refused, `"bob"`) {
			t.Fatalf("the cluster refused %q, want it to name bob", refused)
		}
	})

	t.Run("logs", func(t *testing.T) {
		carol := signIn(t, "carol")
		reply := subscribe(t, carol, logsFor("default", defaultPod))
		if !strings.Contains(reply.Message, `"carol"`) {
			t.Fatalf("the cluster refused %q, want it to name carol", reply.Message)
		}
		mine := subscribe(t, carol, logsFor("payments", paymentsPod))
		if mine.Type == "error" {
			t.Fatalf("carol could not read logs in her own namespace: %s", mine.Message)
		}
	})

	t.Run("helm", func(t *testing.T) {
		installProbe(t, "default")
		defer removeProbe(t, "default")
		bob := signIn(t, "bob")
		status, message := post(t, bob, "/api/helm/action?action=uninstall&namespace=default&name=probe")
		if status < 400 {
			t.Fatalf("bob uninstalled a release where he only reads: %d %s", status, message)
		}
		if !strings.Contains(messageOf(t, message), "bob") {
			t.Fatalf("helm refused %q, want it to name bob", message)
		}

		installProbe(t, "payments")
		alice := signIn(t, "alice")
		status, message = post(t, alice, "/api/helm/action?action=uninstall&namespace=payments&name=probe")
		if status != http.StatusOK {
			t.Fatalf("alice could not uninstall a release: %d %s", status, message)
		}
	})
}
