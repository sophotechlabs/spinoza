package server

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const openAPIPath = "../../docs/api/openapi.yaml"

var (
	openAPIPathLine   = regexp.MustCompile(`^ {2}(/\S*):$`)
	openAPIMethodLine = regexp.MustCompile(`^ {4}(get|put|post|delete|patch):$`)
	openAPISummary    = regexp.MustCompile(`^ {6}summary: (\S.*)$`)
)

type documented struct {
	method  string
	path    string
	summary string
}

func readOpenAPI(t *testing.T) []documented {
	t.Helper()
	body, err := os.ReadFile(filepath.Clean(openAPIPath))
	if err != nil {
		t.Fatalf("read %s: %v", openAPIPath, err)
	}
	out := []documented{}
	path := ""
	method := ""
	for line := range strings.SplitSeq(string(body), "\n") {
		if found := openAPIPathLine.FindStringSubmatch(line); found != nil {
			path = found[1]
			method = ""
			continue
		}
		if found := openAPIMethodLine.FindStringSubmatch(line); found != nil {
			method = strings.ToUpper(found[1])
			continue
		}
		if found := openAPISummary.FindStringSubmatch(line); found != nil && method != "" {
			out = append(out, documented{method: method, path: path, summary: found[1]})
		}
	}
	return out
}

func servedRoutes(t *testing.T) []documented {
	t.Helper()
	srv := New(&stubBackendCluster{backend: &stubCatalog{}}, testAssets(), testToken)
	srv.UseClusterAuth(ClusterAuth{})
	served := srv.allRoutes()
	out := make([]documented, 0, len(served))
	for _, entry := range served {
		out = append(out, documented{method: entry.method, path: entry.path})
	}
	return out
}

func keyOf(one documented) string {
	return one.method + " " + one.path
}

func TestEveryRouteIsDescribedInTheOpenAPIDocument(t *testing.T) {
	written := map[string]bool{}
	for _, one := range readOpenAPI(t) {
		written[keyOf(one)] = true
	}

	missing := []string{}
	for _, one := range servedRoutes(t) {
		if !written[keyOf(one)] {
			missing = append(missing, keyOf(one))
		}
	}

	if len(missing) > 0 {
		slices.Sort(missing)
		t.Fatalf(
			"these routes are served and not described in %s:\n\t%s",
			openAPIPath, strings.Join(missing, "\n\t"),
		)
	}
}

func TestTheOpenAPIDocumentDescribesNothingSpinozaDoesNotServe(t *testing.T) {
	served := map[string]bool{}
	for _, one := range servedRoutes(t) {
		served[keyOf(one)] = true
	}

	extra := []string{}
	for _, one := range readOpenAPI(t) {
		if !served[keyOf(one)] {
			extra = append(extra, keyOf(one))
		}
	}

	if len(extra) > 0 {
		slices.Sort(extra)
		t.Fatalf(
			"%s describes these, and spinoza serves none of them:\n\t%s",
			openAPIPath, strings.Join(extra, "\n\t"),
		)
	}
}

func TestEveryDescribedRouteSaysWhatItIsFor(t *testing.T) {
	for _, one := range readOpenAPI(t) {
		if len(one.summary) < 8 {
			t.Errorf("%s is described as %q, which says nothing", keyOf(one), one.summary)
		}
		if strings.HasSuffix(one.summary, ".") {
			t.Errorf("%s ends its summary with a full stop; the others do not", keyOf(one))
		}
	}
}
