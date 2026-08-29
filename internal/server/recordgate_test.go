package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var writesToTheCluster = map[string]string{
	"/api/action":        "scale, restart, cordon, drain and the rest",
	"/api/object":        "apply and delete write objects",
	"/api/helm/action":   "rollback and uninstall",
	"/api/helm/upgrade":  "upgrades a release",
	"/api/helm/install":  "installs a release",
	"/api/flux/action":   "reconcile, suspend, resume",
	"/api/argocd/action": "sync, refresh, rollback",
	"/api/exec":          "opens a shell in a container",
	"/api/nodeshell":     "creates a privileged pod",
	"/api/debug":         "attaches an ephemeral container",
}

var localOnly = map[string]string{
	"/api/update":             "installs a spinoza update on this machine",
	"/api/contexts":           "switches which cluster is looked at",
	"/api/protection":         "a local flag about a cluster, not a change to it",
	"/api/kubeconfigs":        "edits this machine's kubeconfig list",
	"/api/kubeconfigs/picker": "opens a file dialog",
	"/api/resources":          "re-reads discovery",
	"/api/access":             "asks the apiserver what it would allow",
	"/api/portforward":        "a local tunnel",
	"/api/clusters":           "opens and closes local connections",
	"/api/clusters/active":    "picks which open connection is active",
	"/api/clusters/color":     "a local label for a tab, not a change to the cluster",
	"/api/history":            "clears the local record",
	"/api/view/browser":       "moves the window",
	"/api/view/desktop":       "moves the window",
	"/api/settings":           "writes local settings",
}

func mutatingPaths(t *testing.T) []string {
	t.Helper()
	server := &Server{}
	seen := map[string]bool{}
	out := []string{}
	for _, route := range server.routes() {
		if route.method == http.MethodGet {
			continue
		}
		if seen[route.path] {
			continue
		}
		seen[route.path] = true
		out = append(out, route.path)
	}
	slices.Sort(out)
	return out
}

func TestEveryMutatingRouteIsClassified(t *testing.T) {
	for _, path := range mutatingPaths(t) {
		_, writes := writesToTheCluster[path]
		_, local := localOnly[path]
		if writes && local {
			t.Errorf("%s is in both lists; it changes the cluster or it does not", path)
			continue
		}
		if !writes && !local {
			t.Errorf(
				"%s takes a write method and is in neither list. If it changes the cluster it "+
					"must call s.record; if it only touches this machine, say so in localOnly.",
				path,
			)
		}
	}
}

func TestEveryClusterChangingRouteRecordsWhatItDid(t *testing.T) {
	recorded := handlersThatRecord(t)
	for path := range writesToTheCluster {
		if !recorded[path] {
			t.Errorf(
				"%s changes the cluster but its handler never calls s.record, so the change "+
					"lands with nothing in History", path,
			)
		}
	}
}

func handlersThatRecord(t *testing.T) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	files := serverFiles(t, fset)

	bodies := map[string][]string{}
	records := map[string]bool{}
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			called := calledNames(fn)
			bodies[fn.Name.Name] = called
			if slices.Contains(called, "record") {
				records[fn.Name.Name] = true
			}
		}
	}
	for range len(bodies) {
		grew := false
		for name, called := range bodies {
			if records[name] {
				continue
			}
			for _, one := range called {
				if records[one] {
					records[name] = true
					grew = true
					break
				}
			}
		}
		if !grew {
			break
		}
	}

	out := map[string]bool{}
	for path, handler := range routeHandlers(t) {
		out[path] = records[handler]
	}
	return out
}

func calledNames(fn *ast.FuncDecl) []string {
	out := []string{}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.SelectorExpr:
			out = append(out, fun.Sel.Name)
		case *ast.Ident:
			out = append(out, fun.Name)
		default:
		}
		return true
	})
	return out
}

var routeLine = regexp.MustCompile(
	`\{http\.Method\w+, "([^"]+)", (?:withRef\()?s\.(\w+)`,
)

func routeHandlers(t *testing.T) map[string]string {
	t.Helper()
	source, err := os.ReadFile("routes.go")
	if err != nil {
		t.Fatalf("read routes.go: %v", err)
	}
	out := map[string]string{}
	for _, found := range routeLine.FindAllStringSubmatch(string(source), -1) {
		out[found[1]] = found[2]
	}
	return out
}

func serverFiles(t *testing.T, fset *token.FileSet) []*ast.File {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the server package: %v", err)
	}
	out := []*ast.File{}
	for _, one := range entries {
		name := one.Name()
		if one.IsDir() || strings.HasSuffix(name, "_test.go") || filepath.Ext(name) != ".go" {
			continue
		}
		parsed, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		out = append(out, parsed)
	}
	return out
}
