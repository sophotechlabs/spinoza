package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const nativeImport = `{"tool":"myscan","findings":[
 {"id":"no-latest","title":"Image tag is latest","severity":"medium","category":"security",
  "wrong":"A floating tag means the running image can change under the deployment.",
  "remedy":"Pin the image to a digest or a version.",
  "detail":"container app runs registry.example/app:latest","container":"app",
  "object":{"apiVersion":"apps/v1","kind":"Deployment","namespace":"apps","name":"api"}},
 {"id":"no-latest","detail":"container web runs registry.example/web:latest","container":"web",
  "object":{"apiVersion":"apps/v1","kind":"Deployment","namespace":"apps","name":"ghost"}}
]}`

func importing(t *testing.T, paths []string, objects ...*unstructured.Unstructured) api.CheckReport {
	t.Helper()
	keep := wholeCluster()
	keep.Imports = paths
	return Run(t.Context(), newLister(objects...), descriptors(), api.Metrics{}, keep, 0)
}

func writeImport(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
	return path
}

func fixtureImport(name string) string {
	return filepath.Join("testdata", "imports", name)
}

func clusterWide(kind, apiVersion, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   map[string]any{"name": name},
	}}
}

func namespacedObj(kind, apiVersion, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   metadata(name),
	}}
}

func takenAt(t *testing.T, path string) string {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.ModTime().UTC().Format(time.RFC3339)
}

// what a file's finding becomes in the report

func TestAnImportedFindingLandsOnTheObjectItNames(t *testing.T) {
	path := writeImport(t, "myscan.json", nativeImport)

	found := importing(t, []string{path}, deployment("api", podSpec(container("app", nil))))

	group := groupNamed(t, found, "myscan/no-latest")
	if group.Title != "Image tag is latest" || group.Severity != severityMedium || group.Category != categorySecurity {
		t.Fatalf("group = %+v, want the file's title, severity and category", group)
	}
	if !strings.HasPrefix(group.Wrong, "A floating tag") || !strings.HasPrefix(group.Remedy, "Pin the image") {
		t.Fatalf("wrong = %q, remedy = %q, want the file's words", group.Wrong, group.Remedy)
	}
	if len(group.Sources) != 1 || group.Sources[0] != path || group.Taken != takenAt(t, path) {
		t.Fatalf("sources = %v taken = %q, want the file and when it was written", group.Sources, group.Taken)
	}
	if len(group.Findings) != 2 {
		t.Fatalf("findings = %d, want the matched and the unmatched one", len(group.Findings))
	}
	matched := group.Findings[0]
	object := objectFor(t, found, matched)
	if object.Name != "api" || object.Kind != "Deployment" || object.Resource != "deployments" || object.Namespace != testNamespace {
		t.Fatalf("matched object = %+v, want the deployment the audit read", object)
	}
	if matched.Unmatched || matched.Container != "app" || matched.Detail != "container app runs registry.example/app:latest" {
		t.Fatalf("matched finding = %+v", matched)
	}
	ghost := group.Findings[1]
	if !ghost.Unmatched {
		t.Fatal("a finding about an object the audit never saw was not marked")
	}
	if shape := objectFor(t, found, ghost); shape.Name != "ghost" || shape.Kind != "Deployment" || shape.Resource != "" {
		t.Fatalf("unmatched object = %+v, want the identity the file gave and no resource", shape)
	}
}

func TestAnImportedFindingRanksByReachLikeABuiltInOne(t *testing.T) {
	path := writeImport(t, "myscan.json", nativeImport)

	found := importing(t, []string{path}, replicas(deployment("api", podSpec(container("app", nil))), 3))

	group := groupNamed(t, found, "myscan/no-latest")
	if group.Findings[0].Severity != severityHigh {
		t.Fatalf("a finding on a three-replica deployment ranked %q, want high like a built-in check", group.Findings[0].Severity)
	}
	if group.Findings[1].Severity != severityMedium {
		t.Fatalf("an unmatched finding ranked %q, want the file's own medium with no reach applied", group.Findings[1].Severity)
	}
}

func TestImportedGroupsComeAfterTheBuiltInAndPersonalOnes(t *testing.T) {
	path := writeImport(t, "myscan.json", nativeImport)

	found := importing(t, []string{path})

	if last := found.Groups[len(found.Groups)-1].ID; last != "myscan/no-latest" {
		t.Fatalf("last group = %s, want the imported one", last)
	}
}

func TestAnImportedRuleCanBeDisabledLikeAnyOther(t *testing.T) {
	path := writeImport(t, "myscan.json", nativeImport)
	keep := wholeCluster()
	keep.Imports = []string{path}
	keep.Disabled = []string{"myscan/no-latest"}

	found := Run(t.Context(), newLister(), descriptors(), api.Metrics{}, keep, 0)

	for _, group := range found.Groups {
		if group.ID == "myscan/no-latest" {
			t.Fatal("a disabled imported rule was still reported")
		}
	}
}

func TestTwoFilesFromOneToolShareARule(t *testing.T) {
	first := writeImport(t, "first.json", nativeImport)
	second := writeImport(t, "second.json", `{"tool":"myscan","findings":[{"id":"no-latest","detail":"container job runs registry.example/job:latest","object":{"kind":"Job","namespace":"apps","name":"nightly"}}]}`)
	older := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	if err := os.Chtimes(first, older, older); err != nil {
		t.Fatal(err)
	}

	found := importing(t, []string{second, first})

	group := groupNamed(t, found, "myscan/no-latest")
	if len(group.Sources) != 2 || group.Sources[0] != second || group.Sources[1] != first {
		t.Fatalf("sources = %v, want both files in the order they were listed", group.Sources)
	}
	if group.Taken != older.UTC().Format(time.RFC3339) {
		t.Fatalf("taken = %q, want the older file's time %s", group.Taken, older.UTC().Format(time.RFC3339))
	}
	if len(group.Findings) != 3 {
		t.Fatalf("findings = %d, want the three from both files", len(group.Findings))
	}
}

// how the file is read

func TestAnImportIsReadAgainOnceTheFileChanges(t *testing.T) {
	path := writeImport(t, "myscan.json", `{"tool":"myscan","findings":[{"id":"no-latest","object":{"kind":"Deployment","namespace":"apps","name":"api"}}]}`)
	if len(groupNamed(t, importing(t, []string{path}), "myscan/no-latest").Findings) != 1 {
		t.Fatal("the first read did not report the one finding")
	}

	if err := os.WriteFile(path, []byte(nativeImport), 0o600); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}

	if got := len(groupNamed(t, importing(t, []string{path}), "myscan/no-latest").Findings); got != 2 {
		t.Fatalf("after the file changed the report carried %d findings, want 2", got)
	}
}

func TestAnImportNobodyCanReadIsNamedInTheReport(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone.json")
	folder := t.TempDir()
	huge := filepath.Join(t.TempDir(), "huge.json")
	if err := os.WriteFile(huge, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(huge, maxImportBytes+1); err != nil {
		t.Fatal(err)
	}

	found := importing(t, []string{missing, folder, huge}, deployment("api", podSpec(container("app", nil))))

	for _, want := range []string{
		"import " + missing + ": ",
		"import " + folder + ": not a regular file",
		"import " + huge + ": larger than 32 MiB",
	} {
		if !strings.Contains(found.Error, want) {
			t.Fatalf("error = %q, want it to carry %q", found.Error, want)
		}
	}
	if findingCount(t, found, "privileged-containers") != 0 || len(found.Groups) != len(registry()) {
		t.Fatal("an unreadable import disturbed the built-in audit")
	}
}

func TestAnImportInAFormatSpinozaDoesNotReadIsNamed(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "unknown object", body: `{"hello": 1}`, want: "not a format spinoza reads"},
		{name: "not json", body: `hello`, want: "not a JSON object"},
		{name: "finding without identity", body: `{"findings":[{"id":"x"}]}`, want: "finding 1 needs an id and an object with a kind and a name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeImport(t, "odd.json", tc.body)
			found := importing(t, []string{path})
			if !strings.Contains(found.Error, "import "+path+": "+tc.want) {
				t.Fatalf("error = %q, want %q", found.Error, tc.want)
			}
		})
	}
}

func TestParseImportsKeepsOnePathPerLine(t *testing.T) {
	got := ParseImports(" /a.json \n\n# a note\n/a.json\n/b.json\r\n")
	if len(got) != 2 || got[0] != "/a.json" || got[1] != "/b.json" {
		t.Fatalf("paths = %v", got)
	}
	many := make([]string, 0, 40)
	for at := range 40 {
		many = append(many, "/file-"+itoa(at)+".json")
	}
	if got := ParseImports(strings.Join(many, "\n")); len(got) != maxImportPaths {
		t.Fatalf("%d paths kept, want the cap of %d", len(got), maxImportPaths)
	}
	if got := ParseImports("  \n"); len(got) != 0 {
		t.Fatalf("blank input gave %v", got)
	}
}

// the formats scanners write

func TestATrivyKubernetesReportImportsWhatFailed(t *testing.T) {
	found := importing(t, []string{fixtureImport("trivy-k8s.json")},
		deployment("api", podSpec(container("app", nil))),
		clusterWide("ClusterRole", "rbac.authorization.k8s.io/v1", "admin"))

	escalation := groupNamed(t, found, "trivy/KSV-0001")
	if escalation.Title != "Can elevate its own privileges" || escalation.Severity != severityMedium {
		t.Fatalf("group = %+v", escalation)
	}
	if !strings.HasPrefix(escalation.Wrong, "A program inside the container") || !strings.HasPrefix(escalation.Remedy, "Set 'set containers[]") {
		t.Fatalf("wrong = %q remedy = %q, want trivy's description and resolution", escalation.Wrong, escalation.Remedy)
	}
	finding := onlyFinding(t, found, "trivy/KSV-0001")
	if finding.Unmatched || objectFor(t, found, finding).Name != "api" {
		t.Fatalf("finding = %+v, want it on the deployment the audit read", finding)
	}
	if finding.Detail != "Container 'app' of Deployment 'api' should set 'securityContext.allowPrivilegeEscalation' to false" {
		t.Fatalf("detail = %q, want trivy's message", finding.Detail)
	}

	secrets := groupNamed(t, found, "trivy/KSV-0041")
	if secrets.Severity != severityHigh {
		t.Fatalf("a CRITICAL trivy finding ranked %q, want high", secrets.Severity)
	}
	if role := onlyObject(t, found, "trivy/KSV-0041"); role.Kind != "ClusterRole" || role.Name != "admin" || role.Resource != "clusterroles" {
		t.Fatalf("object = %+v, want the cluster role the audit read", role)
	}

	if low := groupNamed(t, found, "trivy/KSV-0113"); low.Severity != severityLow {
		t.Fatalf("a LOW trivy finding ranked %q, want low", low.Severity)
	}
	for _, group := range found.Groups {
		if group.ID == "trivy/KSV-0003" {
			t.Fatal("a misconfiguration trivy marked PASS was imported")
		}
	}
}

func TestAFileWhoseShapeLiesAboutItsFormatIsNamed(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "native", body: `{"findings": 5}`},
		{name: "trivy", body: `{"Resources": 5}`},
		{name: "kubescape", body: `{"summaryDetails": {}, "results": 5}`},
		{name: "sarif", body: `{"runs": 5}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeImport(t, "odd.json", tc.body)
			found := importing(t, []string{path})
			if !strings.Contains(found.Error, "import "+path+": ") || !strings.Contains(found.Error, "cannot unmarshal") {
				t.Fatalf("error = %q, want the decode failure named", found.Error)
			}
		})
	}
}

func TestAnImportWithTooManyRulesOrFindingsIsRefused(t *testing.T) {
	rules := make([]string, 0, maxImportRules+1)
	for at := range maxImportRules + 1 {
		rules = append(rules, `{"id":"r`+itoa(at)+`","object":{"kind":"Deployment","namespace":"apps","name":"api"}}`)
	}
	tooManyRules := writeImport(t, "rules.json", `{"findings":[`+strings.Join(rules, ",")+`]}`)
	items := make([]string, 0, maxImportItems+1)
	for range maxImportItems + 1 {
		items = append(items, `{"id":"r","object":{"kind":"Deployment","namespace":"apps","name":"api"}}`)
	}
	tooManyItems := writeImport(t, "items.json", `{"findings":[`+strings.Join(items, ",")+`]}`)

	found := importing(t, []string{tooManyRules, tooManyItems})

	if !strings.Contains(found.Error, "import "+tooManyRules+": more than "+itoa(maxImportRules)+" distinct rules") {
		t.Fatalf("error = %q, want the rule cap named", found.Error)
	}
	if !strings.Contains(found.Error, "import "+tooManyItems+": more than "+itoa(maxImportItems)+" findings") {
		t.Fatalf("error = %q, want the finding cap named", found.Error)
	}
}

func TestAKubescapeReportImportsItsFailedControls(t *testing.T) {
	found := importing(t, []string{fixtureImport("kubescape.json")},
		deployment("api", podSpec(container("app", nil))),
		namespacedObj("ServiceAccount", "v1", "robot"))

	escalation := groupNamed(t, found, "kubescape/C-0016")
	if escalation.Title != "Allow privilege escalation" || escalation.Severity != severityMedium {
		t.Fatalf("group = %+v", escalation)
	}
	if !strings.Contains(escalation.Wrong, "C-0016") || !strings.Contains(escalation.Remedy, "hub.armosec.io/docs/c-0016") {
		t.Fatalf("wrong = %q remedy = %q", escalation.Wrong, escalation.Remedy)
	}
	if finding := onlyFinding(t, found, "kubescape/C-0016"); finding.Unmatched || objectFor(t, found, finding).Name != "api" {
		t.Fatalf("finding = %+v, want it on the deployment", finding)
	}
	if account := onlyObject(t, found, "kubescape/C-0053"); account.Kind != "ServiceAccount" || account.Name != "robot" {
		t.Fatalf("object = %+v, want the service account from the composite resource", account)
	}
	for _, group := range found.Groups {
		if group.ID == "kubescape/C-0002" {
			t.Fatal("a control kubescape marked passed was imported")
		}
	}
}

func TestASARIFFileIsReadAsFarAsItNamesObjects(t *testing.T) {
	found := importing(t, []string{fixtureImport("trivy-config.sarif")},
		deployment("api", podSpec(container("app", nil))),
		deployment("twin", podSpec(container("app", nil))),
		namespaced(deployment("twin", podSpec(container("app", nil))), "other"))

	group := groupNamed(t, found, "trivy/KSV-0001")
	if group.Title != "Can elevate its own privileges" || group.Severity != severityMedium || group.Remedy != "Misconfiguration KSV-0001" {
		t.Fatalf("group = %+v", group)
	}
	if len(group.Findings) != 3 {
		t.Fatalf("findings = %d, want 3", len(group.Findings))
	}
	byName := map[string]api.CheckFinding{}
	for _, finding := range group.Findings {
		byName[objectFor(t, found, finding).Name] = finding
	}
	apiFinding := byName["api"]
	if apiFinding.Unmatched || apiFinding.Detail != "Container 'app' of Deployment 'api' should set 'securityContext.allowPrivilegeEscalation' to false" {
		t.Fatalf("a result naming one deployment = %+v, want it matched by kind and name", apiFinding)
	}
	if twin := byName["twin"]; !twin.Unmatched {
		t.Fatal("a name two namespaces share was matched to one of them")
	}
	if file := byName["other.yaml"]; !file.Unmatched || objectFor(t, found, file).Kind != "" {
		t.Fatalf("a result naming no object = %+v, want it kept under its file name", file)
	}
}

func TestASARIFLogicalLocationNamesTheObjectDirectly(t *testing.T) {
	path := writeImport(t, "tool.sarif", `{"runs":[{"tool":{"driver":{"name":"scanner","rules":[]}},"results":[
	 {"ruleId":"R1","level":"error","message":{"text":"broken"},"locations":[{"logicalLocations":[{"fullyQualifiedName":"Deployment/apps/api"}]}]},
	 {"ruleId":"R1","level":"error","message":{"text":"broken"},"locations":[{"logicalLocations":[{"fullyQualifiedName":"Deployment/api"}]}]},
	 {"ruleId":"R1","level":"error","message":{"text":"Deployment '' is unnamed"},"locations":[{"logicalLocations":[{"fullyQualifiedName":"nonsense"}]}]},
	 {"ruleId":"R2","level":"note","message":{"text":"a remark"},"locations":[]}
	]}]}`)

	found := importing(t, []string{path}, deployment("api", podSpec(container("app", nil))))

	group := groupNamed(t, found, "scanner/R1")
	if group.Severity != severityHigh || group.Title != "R1" {
		t.Fatalf("group = %+v, want error as high and the id as title", group)
	}
	if remark := groupNamed(t, found, "scanner/R2"); remark.Severity != severityLow || onlyFinding(t, found, "scanner/R2").Detail != "a remark" {
		t.Fatalf("a note-level result = %+v", remark)
	}
	matched := 0
	for _, finding := range group.Findings {
		name := objectFor(t, found, finding).Name
		if !finding.Unmatched && name == "api" {
			matched++
			continue
		}
		if !finding.Unmatched || name != "unnamed" {
			t.Fatalf("a location nothing reads = %+v on %q", finding, name)
		}
	}
	if matched != 2 {
		t.Fatalf("%d matched, want Kind/namespace/name and Kind/name both to land on the deployment", matched)
	}
}

func TestAKubescapeResultAboutAResourceTheReportDoesNotListIsDropped(t *testing.T) {
	path := writeImport(t, "ks.json", `{"summaryDetails":{"controls":{}},"resources":[],"results":[
	 {"resourceID":"apps/v1/apps/Deployment/api","controls":[{"controlID":"C-0016","name":"Allow privilege escalation","status":{"status":"failed"},"severity":"Medium"}]}
	]}`)

	found := importing(t, []string{path}, deployment("api", podSpec(container("app", nil))))

	if found.Error != "" {
		t.Fatalf("error = %q", found.Error)
	}
	for _, group := range found.Groups {
		if group.ID == "kubescape/C-0016" {
			t.Fatal("a result with no resource entry to identify it became a finding")
		}
	}
}
