//go:build clustermode

package clustermode

import (
	"fmt"
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

func restart(namespace, name string) string {
	query := url.Values{
		"action":    {"restart"},
		"group":     {"apps"},
		"version":   {"v1"},
		"resource":  {"deployments"},
		"namespace": {namespace},
		"name":      {name},
	}
	return "/api/action?" + query.Encode()
}

func objectPath(namespace, name string) string {
	query := url.Values{
		"version":   {"v1"},
		"resource":  {"configmaps"},
		"namespace": {namespace},
		"name":      {name},
	}
	return "/api/object?" + query.Encode()
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

	defaultPod := podIn(t, "default", "app=other")

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
		defer func() {
			kubectl(t, "-n", "payments", "scale", "deployment/web", "--replicas=1")
			kubectl(t, "-n", "payments", "rollout", "status", "deployment/web", "--timeout=5m")
		}()
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

	t.Run("a restart", func(t *testing.T) {
		bob := signIn(t, "bob")
		status, message := post(t, bob, restart("payments", "web"))
		if status != http.StatusOK {
			t.Fatalf("restarting in its own namespace gave %d: %s", status, message)
		}
		kubectl(t, "-n", "payments", "rollout", "status", "deployment/web", "--timeout=5m")
		status, message = post(t, bob, restart("default", "other"))
		if status != http.StatusForbidden {
			t.Fatalf("restarting elsewhere gave %d: %s", status, message)
		}
		if !strings.Contains(messageOf(t, message), `User "bob"`) {
			t.Fatalf("the cluster refused %q, want it to name bob", message)
		}
	})

	t.Run("an apply and delete", func(t *testing.T) {
		name := "cluster-mode-edit"
		defer func() {
			_, _ = maybeKubectl(t, "-n", "payments", "delete", "configmap", name, "--ignore-not-found=true")
			_, _ = maybeKubectl(t, "-n", "default", "delete", "configmap", name, "--ignore-not-found=true")
		}()
		_, _ = maybeKubectl(t, "-n", "payments", "delete", "configmap", name, "--ignore-not-found=true")
		_, _ = maybeKubectl(t, "-n", "default", "delete", "configmap", name, "--ignore-not-found=true")
		kubectl(t, "-n", "payments", "create", "configmap", name, "--from-literal=message=before")
		kubectl(t, "-n", "default", "create", "configmap", name, "--from-literal=message=before")
		allowedResourceVersion := strings.TrimSpace(kubectl(t, "-n", "payments", "get", "configmap", name,
			"-o", "jsonpath={.metadata.resourceVersion}"))
		deniedResourceVersion := strings.TrimSpace(kubectl(t, "-n", "default", "get", "configmap", name,
			"-o", "jsonpath={.metadata.resourceVersion}"))
		bob := signIn(t, "bob")
		payload := fmt.Sprintf(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"%s","namespace":"payments","resourceVersion":"%s"},"data":{"message":"from-bob"}}`, name, allowedResourceVersion)
		status, message := put(t, bob, objectPath("payments", name), payload)
		if status != http.StatusOK {
			t.Fatalf("applying in its own namespace gave %d: %s", status, message)
		}
		stored := kubectl(t, "-n", "payments", "get", "configmap", name, "-o", "jsonpath={.data.message}")
		if stored != "from-bob" {
			t.Fatalf("stored message = %q, want from-bob", stored)
		}
		status, message = delete(t, bob, objectPath("payments", name))
		if status != http.StatusNoContent {
			t.Fatalf("deleting in its own namespace gave %d: %s", status, message)
		}
		deniedPayload := fmt.Sprintf(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"%s","namespace":"default","resourceVersion":"%s"},"data":{"message":"from-bob"}}`, name, deniedResourceVersion)
		status, message = put(t, bob, objectPath("default", name), deniedPayload)
		if status != http.StatusForbidden {
			t.Fatalf("applying elsewhere gave %d: %s", status, message)
		}
		if !strings.Contains(messageOf(t, message), `User "bob"`) {
			t.Fatalf("the cluster refused %q, want it to name bob", message)
		}
	})

	t.Run("a shell", func(t *testing.T) {
		paymentsPod := podIn(t, "payments", "app=web")
		opened := shell(t, signIn(t, "alice"), "/api/exec?namespace=payments&pod="+paymentsPod)
		if !strings.HasPrefix(opened, "OPENED") {
			t.Fatalf("alice could not open a shell: %s", opened)
		}
		refused := shell(t, signIn(t, "bob"), "/api/exec?namespace=default&pod="+defaultPod)
		if !strings.Contains(refused, "refused before the shell opened (403)") {
			t.Fatalf("the cluster refused %q, want an HTTP 403", refused)
		}
		if !strings.Contains(refused, "you may not create pods/exec here") {
			t.Fatalf("the cluster refused %q, want an exec authorization denial", refused)
		}
	})

	t.Run("a port forward, which would land on the server rather than on anyone", func(t *testing.T) {
		paymentsPod := podIn(t, "payments", "app=web")
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
		if !strings.Contains(refused, "refused before the shell opened (403)") {
			t.Fatalf("the cluster refused %q, want an HTTP 403", refused)
		}
		if !strings.Contains(refused, "you may not create pods in kube-system") {
			t.Fatalf("the cluster refused %q, want a node-shell authorization denial", refused)
		}
	})

	t.Run("logs", func(t *testing.T) {
		paymentsPod := podIn(t, "payments", "app=web")
		carol := signIn(t, "carol")
		reply := subscribe(t, carol, logsFor("default", defaultPod))
		if reply.Type != "error" {
			t.Fatalf("logs in another namespace returned %q, want an error", reply.Type)
		}
		if !strings.Contains(reply.Message, "you may not get pods/log here") {
			t.Fatalf("the cluster refused %q, want a log authorization denial", reply.Message)
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

	t.Run("helm rollback and uninstall", func(t *testing.T) {
		installProbe(t, "payments")
		defer func() {
			removeProbe(t, "payments")
		}()
		run(t, "helm", "--kube-context", context1(t), "upgrade", "probe", "chart",
			"--namespace", "payments", "--set", "message=changed", "--wait", "--timeout", "2m")
		bob := signIn(t, "bob")
		status, message := post(t, bob,
			"/api/helm/action?action=rollback&revision=1&namespace=payments&name=probe")
		if status != http.StatusOK {
			t.Fatalf("bob could not roll back a release in payments: %d %s", status, message)
		}
		stored := kubectl(t, "-n", "payments", "get", "configmap", "probe", "-o", "jsonpath={.data.message}")
		if stored != "hello" {
			t.Fatalf("rolled back message = %q, want hello", stored)
		}
		status, message = post(t, bob,
			"/api/helm/action?action=uninstall&namespace=payments&name=probe")
		if status != http.StatusOK {
			t.Fatalf("bob could not uninstall a release in payments: %d %s", status, message)
		}
	})
}
