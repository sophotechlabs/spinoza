package checks

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const (
	ImportsKey     = "spinoza.checks.imports.v1"
	maxImportPaths = 32
	maxImportBytes = 32 << 20
	maxImportRules = 2000
	maxImportItems = 100000
)

var errImportTooLarge = errors.New("larger than 32 MiB")

func ParseImports(raw string) []string {
	out := []string{}
	for line := range strings.SplitSeq(raw, "\n") {
		path := strings.TrimSpace(line)
		if path == "" || strings.HasPrefix(path, "#") || slices.Contains(out, path) {
			continue
		}
		if len(out) == maxImportPaths {
			break
		}
		out = append(out, path)
	}
	return out
}

type importedObject struct {
	group     string
	version   string
	kind      string
	namespace string
	name      string
}

type importedFinding struct {
	object    importedObject
	container string
	detail    string
}

type importedRule struct {
	id       string
	title    string
	category string
	severity string
	wrong    string
	remedy   string
	findings []importedFinding
}

type importFile struct {
	path  string
	tool  string
	taken time.Time
	rules []importedRule
}

type importReader func(raw []byte) (string, []importedRule, error)

type cachedImport struct {
	modified time.Time
	size     int64
	file     importFile
	err      error
}

var importCache = struct {
	mu   sync.Mutex
	seen map[string]cachedImport
}{seen: map[string]cachedImport{}}

func readImports(paths []string) ([]importFile, []string) {
	files := []importFile{}
	faults := []string{}
	for _, path := range paths {
		file, err := readImport(path)
		if err != nil {
			faults = append(faults, "import "+path+": "+err.Error())
			continue
		}
		files = append(files, file)
	}
	return files, faults
}

func readImport(path string) (importFile, error) {
	info, err := os.Stat(path)
	if err != nil {
		return importFile{}, err
	}
	if !info.Mode().IsRegular() {
		return importFile{}, errors.New("not a regular file")
	}
	if info.Size() > maxImportBytes {
		return importFile{}, errImportTooLarge
	}
	importCache.mu.Lock()
	defer importCache.mu.Unlock()
	held, seen := importCache.seen[path]
	if seen && held.modified.Equal(info.ModTime()) && held.size == info.Size() {
		return held.file, held.err
	}
	file, err := loadImport(path, info.ModTime())
	importCache.seen[path] = cachedImport{modified: info.ModTime(), size: info.Size(), file: file, err: err}
	return file, err
}

func loadImport(path string, modified time.Time) (importFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return importFile{}, err
	}
	read, err := readerFor(raw)
	if err != nil {
		return importFile{}, err
	}
	tool, rules, err := read(raw)
	if err != nil {
		return importFile{}, err
	}
	if tool == "" {
		tool = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	return importFile{path: path, tool: strings.ToLower(tool), taken: modified, rules: rules}, nil
}

func readerFor(raw []byte) (importReader, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, errors.New("not a JSON object: " + err.Error())
	}
	_, native := top["findings"]
	_, trivy := top["Resources"]
	_, sarif := top["runs"]
	_, kubescape := top["summaryDetails"]
	switch {
	case native:
		return readNative, nil
	case trivy:
		return readTrivy, nil
	case sarif:
		return readSARIF, nil
	case kubescape:
		return readKubescape, nil
	default:
		return nil, errors.New("not a format spinoza reads: spinoza findings, a trivy Kubernetes report, a kubescape report or SARIF")
	}
}

type nativeFile struct {
	Tool     string          `json:"tool"`
	Findings []nativeFinding `json:"findings"`
}

type nativeFinding struct {
	ID        string       `json:"id"`
	Title     string       `json:"title"`
	Category  string       `json:"category"`
	Severity  string       `json:"severity"`
	Wrong     string       `json:"wrong"`
	Remedy    string       `json:"remedy"`
	Detail    string       `json:"detail"`
	Container string       `json:"container"`
	Object    nativeObject `json:"object"`
}

type nativeObject struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
}

func readNative(raw []byte) (string, []importedRule, error) {
	var file nativeFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return "", nil, err
	}
	rules := newRuleSet()
	for at, one := range file.Findings {
		if one.ID == "" || one.Object.Kind == "" || one.Object.Name == "" {
			return "", nil, errors.New("finding " + itoa(at+1) + " needs an id and an object with a kind and a name")
		}
		rule := rules.add(importedRule{
			id:       one.ID,
			title:    firstOf(one.Title, one.ID),
			category: knownCategory(one.Category),
			severity: knownSeverity(one.Severity),
			wrong:    one.Wrong,
			remedy:   one.Remedy,
		})
		group, version := splitAPIVersion(one.Object.APIVersion)
		rule.findings = append(rule.findings, importedFinding{
			object: importedObject{
				group:     group,
				version:   version,
				kind:      one.Object.Kind,
				namespace: one.Object.Namespace,
				name:      one.Object.Name,
			},
			container: one.Container,
			detail:    firstOf(one.Detail, one.Wrong, one.Title, one.ID),
		})
		if err := rules.bounded(); err != nil {
			return "", nil, err
		}
	}
	return file.Tool, rules.list(), nil
}

type trivyReport struct {
	Resources []trivyResource `json:"Resources"`
}

type trivyResource struct {
	Namespace string        `json:"Namespace"`
	Kind      string        `json:"Kind"`
	Name      string        `json:"Name"`
	Results   []trivyResult `json:"Results"`
}

type trivyResult struct {
	Misconfigurations []trivyMisconfiguration `json:"Misconfigurations"`
}

type trivyMisconfiguration struct {
	ID          string `json:"ID"`
	Title       string `json:"Title"`
	Description string `json:"Description"`
	Message     string `json:"Message"`
	Resolution  string `json:"Resolution"`
	Severity    string `json:"Severity"`
	Status      string `json:"Status"`
}

func readTrivy(raw []byte) (string, []importedRule, error) {
	var report trivyReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return "", nil, err
	}
	rules := newRuleSet()
	for _, resource := range report.Resources {
		for _, result := range resource.Results {
			for _, one := range result.Misconfigurations {
				if one.Status != "" && one.Status != "FAIL" {
					continue
				}
				rule := rules.add(importedRule{
					id:       one.ID,
					title:    firstOf(one.Title, one.ID),
					category: categorySecurity,
					severity: severityFromScanner(one.Severity),
					wrong:    one.Description,
					remedy:   one.Resolution,
				})
				rule.findings = append(rule.findings, importedFinding{
					object: importedObject{kind: resource.Kind, namespace: resource.Namespace, name: resource.Name},
					detail: firstOf(one.Message, one.Title),
				})
				if err := rules.bounded(); err != nil {
					return "", nil, err
				}
			}
		}
	}
	return "trivy", rules.list(), nil
}

type kubescapeReport struct {
	Resources []kubescapeResource `json:"resources"`
	Results   []kubescapeResult   `json:"results"`
	Summary   kubescapeSummary    `json:"summaryDetails"`
}

type kubescapeSummary struct {
	Controls map[string]kubescapeControlSummary `json:"controls"`
}

type kubescapeControlSummary struct {
	Name     string `json:"name"`
	Severity string `json:"severity"`
}

type kubescapeResource struct {
	ResourceID string          `json:"resourceID"`
	Object     kubescapeObject `json:"object"`
}

type kubescapeObject struct {
	APIVersion string            `json:"apiVersion"`
	Kind       string            `json:"kind"`
	Namespace  string            `json:"namespace"`
	Name       string            `json:"name"`
	Metadata   kubescapeMetadata `json:"metadata"`
}

type kubescapeMetadata struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

type kubescapeResult struct {
	ResourceID string             `json:"resourceID"`
	Controls   []kubescapeControl `json:"controls"`
}

type kubescapeControl struct {
	ControlID string          `json:"controlID"`
	Name      string          `json:"name"`
	Severity  string          `json:"severity"`
	Status    kubescapeStatus `json:"status"`
}

type kubescapeStatus struct {
	Status string `json:"status"`
}

func readKubescape(raw []byte) (string, []importedRule, error) {
	var report kubescapeReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return "", nil, err
	}
	objects := map[string]importedObject{}
	for _, resource := range report.Resources {
		objects[resource.ResourceID] = kubescapeIdentity(resource.Object)
	}
	rules := newRuleSet()
	for _, result := range report.Results {
		object, known := objects[result.ResourceID]
		if !known {
			continue
		}
		for _, control := range result.Controls {
			if control.Status.Status != "failed" {
				continue
			}
			summary := report.Summary.Controls[control.ControlID]
			name := firstOf(control.Name, summary.Name, control.ControlID)
			rule := rules.add(importedRule{
				id:       control.ControlID,
				title:    name,
				category: categorySecurity,
				severity: severityFromScanner(firstOf(control.Severity, summary.Severity)),
				wrong:    "kubescape control " + control.ControlID + " failed: " + name + ".",
				remedy:   "See https://hub.armosec.io/docs/" + strings.ToLower(control.ControlID) + " for what the control expects.",
			})
			rule.findings = append(rule.findings, importedFinding{object: object, detail: name})
			if err := rules.bounded(); err != nil {
				return "", nil, err
			}
		}
	}
	return "kubescape", rules.list(), nil
}

func kubescapeIdentity(object kubescapeObject) importedObject {
	group, version := splitAPIVersion(object.APIVersion)
	return importedObject{
		group:     group,
		version:   version,
		kind:      object.Kind,
		namespace: firstOf(object.Metadata.Namespace, object.Namespace),
		name:      firstOf(object.Metadata.Name, object.Name),
	}
}

type sarifFile struct {
	Runs []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name  string      `json:"name"`
	Rules []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string             `json:"id"`
	ShortDescription sarifText          `json:"shortDescription"`
	FullDescription  sarifText          `json:"fullDescription"`
	Help             sarifText          `json:"help"`
	Default          sarifConfiguration `json:"defaultConfiguration"`
}

type sarifText struct {
	Text string `json:"text"`
}

type sarifConfiguration struct {
	Level string `json:"level"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"`
	Message   sarifText       `json:"message"`
	Locations []sarifLocation `json:"locations"`
}

type sarifLocation struct {
	Physical sarifPhysical  `json:"physicalLocation"`
	Logical  []sarifLogical `json:"logicalLocations"`
}

type sarifPhysical struct {
	Artifact sarifArtifact `json:"artifactLocation"`
}

type sarifArtifact struct {
	URI string `json:"uri"`
}

type sarifLogical struct {
	FullyQualifiedName string `json:"fullyQualifiedName"`
	Kind               string `json:"kind"`
}

func readSARIF(raw []byte) (string, []importedRule, error) {
	var file sarifFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return "", nil, err
	}
	rules := newRuleSet()
	tool := ""
	for _, run := range file.Runs {
		tool = firstOf(tool, run.Tool.Driver.Name)
		described := map[string]sarifRule{}
		for _, rule := range run.Tool.Driver.Rules {
			described[rule.ID] = rule
		}
		for _, result := range run.Results {
			meta := described[result.RuleID]
			rule := rules.add(importedRule{
				id:       firstOf(result.RuleID, "unnamed"),
				title:    firstOf(meta.ShortDescription.Text, result.RuleID),
				category: categorySecurity,
				severity: severityFromSARIF(firstOf(result.Level, meta.Default.Level)),
				wrong:    firstOf(meta.FullDescription.Text, meta.ShortDescription.Text),
				remedy:   firstLine(meta.Help.Text),
			})
			rule.findings = append(rule.findings, importedFinding{
				object: sarifIdentity(result),
				detail: messageLine(result.Message.Text),
			})
			if err := rules.bounded(); err != nil {
				return "", nil, err
			}
		}
	}
	return tool, rules.list(), nil
}

func sarifIdentity(result sarifResult) importedObject {
	for _, location := range result.Locations {
		for _, logical := range location.Logical {
			if object, ok := objectFromQualifiedName(logical.FullyQualifiedName); ok {
				return object
			}
		}
	}
	if object, ok := objectFromMessage(result.Message.Text); ok {
		return object
	}
	for _, location := range result.Locations {
		if location.Physical.Artifact.URI != "" {
			return importedObject{name: location.Physical.Artifact.URI}
		}
	}
	return importedObject{name: "unnamed"}
}

func objectFromQualifiedName(name string) (importedObject, bool) {
	parts := strings.Split(name, "/")
	switch len(parts) {
	case 3:
		return importedObject{kind: parts[0], namespace: parts[1], name: parts[2]}, parts[0] != "" && parts[2] != ""
	case 2:
		return importedObject{kind: parts[0], name: parts[1]}, parts[0] != "" && parts[1] != ""
	default:
		return importedObject{}, false
	}
}

func objectFromMessage(text string) (importedObject, bool) {
	for line := range strings.SplitSeq(text, "\n") {
		object, ok := objectInLine(line)
		if ok {
			return object, true
		}
	}
	return importedObject{}, false
}

func objectInLine(line string) (importedObject, bool) {
	words := strings.Fields(line)
	for at := 0; at+1 < len(words); at++ {
		kind := words[at]
		quoted := words[at+1]
		if !isKindWord(kind) || !strings.HasPrefix(quoted, "'") {
			continue
		}
		name := strings.Trim(quoted, "'")
		if name == "" {
			continue
		}
		return importedObject{kind: kind, name: name}, true
	}
	return importedObject{}, false
}

var kindWords = map[string]bool{
	"Pod": true, "Deployment": true, "StatefulSet": true, "DaemonSet": true, "ReplicaSet": true,
	"ReplicationController": true, "Job": true, "CronJob": true, "Service": true, "Ingress": true,
	"ConfigMap": true, "Secret": true, "ServiceAccount": true, "Role": true, "ClusterRole": true,
	"RoleBinding": true, "ClusterRoleBinding": true, "Namespace": true, "NetworkPolicy": true,
	"PersistentVolumeClaim": true, "LimitRange": true, "ResourceQuota": true, "Node": true,
}

func isKindWord(word string) bool {
	return kindWords[word]
}

type ruleSet struct {
	byID    map[string]int
	entries []*importedRule
	items   int
}

func newRuleSet() *ruleSet {
	return &ruleSet{byID: map[string]int{}}
}

func (s *ruleSet) add(rule importedRule) *importedRule {
	s.items++
	at, seen := s.byID[rule.id]
	if seen {
		return s.entries[at]
	}
	s.byID[rule.id] = len(s.entries)
	held := rule
	s.entries = append(s.entries, &held)
	return &held
}

func (s *ruleSet) bounded() error {
	if len(s.entries) > maxImportRules {
		return errors.New("more than " + itoa(maxImportRules) + " distinct rules")
	}
	if s.items > maxImportItems {
		return errors.New("more than " + itoa(maxImportItems) + " findings")
	}
	return nil
}

func (s *ruleSet) list() []importedRule {
	out := make([]importedRule, 0, len(s.entries))
	for _, rule := range s.entries {
		out = append(out, *rule)
	}
	return out
}

func severityFromScanner(raw string) string {
	switch strings.ToLower(raw) {
	case "critical", "high":
		return severityHigh
	case "medium":
		return severityMedium
	default:
		return severityLow
	}
}

func severityFromSARIF(level string) string {
	switch level {
	case "error":
		return severityHigh
	case "warning":
		return severityMedium
	default:
		return severityLow
	}
}

func splitAPIVersion(apiVersion string) (group, version string) {
	group, version, found := strings.Cut(apiVersion, "/")
	if !found {
		return "", group
	}
	return group, version
}

func firstOf(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return strings.TrimSpace(line)
}

func messageLine(text string) string {
	for line := range strings.SplitSeq(text, "\n") {
		if rest, found := strings.CutPrefix(strings.TrimSpace(line), "Message: "); found {
			return rest
		}
	}
	return firstLine(text)
}

type importOrigin struct {
	sources  []string
	taken    time.Time
	findings []importedFinding
}

func importedChecks(files []importFile) []check {
	byID := map[string]int{}
	out := []check{}
	for _, file := range files {
		for _, rule := range file.rules {
			id := file.tool + "/" + rule.id
			at, seen := byID[id]
			if !seen {
				at = len(out)
				byID[id] = at
				out = append(out, check{
					id:       id,
					title:    rule.title,
					category: rule.category,
					severity: firstOf(rule.severity, severityMedium),
					wrong:    firstOf(rule.wrong, rule.title),
					remedy:   firstOf(rule.remedy, "See the report that found it."),
					imported: &importOrigin{taken: file.taken},
				})
			}
			entry := &out[at]
			if !slices.Contains(entry.imported.sources, file.path) {
				entry.imported.sources = append(entry.imported.sources, file.path)
			}
			if file.taken.Before(entry.imported.taken) {
				entry.imported.taken = file.taken
			}
			entry.imported.findings = append(entry.imported.findings, rule.findings...)
		}
	}
	for at := range out {
		out[at].find = importFinder(out[at].imported.findings)
	}
	return out
}

func importFinder(items []importedFinding) finder {
	return func(sc scan) []found {
		known := identitiesOf(sc)
		out := make([]found, 0, len(items))
		for _, item := range items {
			subject, matched := known.lookup(item.object)
			out = append(out, found{
				subject:   subject,
				container: item.container,
				detail:    item.detail,
				unmatched: !matched,
			})
		}
		return out
	}
}

type identities struct {
	exact  map[string]Subject
	byName map[string][]Subject
}

func identityKey(kind, namespace, name string) string {
	return kind + "\x00" + namespace + "\x00" + name
}

func identitiesOf(sc scan) identities {
	out := identities{exact: map[string]Subject{}, byName: map[string][]Subject{}}
	for _, item := range sc.held.everything() {
		out.remember(subjectFromHeld(item))
	}
	for _, subject := range sc.subjects {
		out.remember(subject)
	}
	return out
}

func (i identities) remember(subject Subject) {
	key := identityKey(subject.Kind, subject.Ref.Namespace, subject.Ref.Name)
	_, seen := i.exact[key]
	i.exact[key] = subject
	if seen {
		return
	}
	loose := identityKey(subject.Kind, "", subject.Ref.Name)
	i.byName[loose] = append(i.byName[loose], subject)
}

func (i identities) lookup(object importedObject) (Subject, bool) {
	if subject, ok := i.exact[identityKey(object.kind, object.namespace, object.name)]; ok {
		return subject, true
	}
	if object.namespace == "" {
		if candidates := i.byName[identityKey(object.kind, "", object.name)]; len(candidates) == 1 {
			return candidates[0], true
		}
	}
	return placeholderSubject(object), false
}

func placeholderSubject(object importedObject) Subject {
	return Subject{
		Ref: api.ObjectRef{
			Group:     object.group,
			Version:   object.version,
			Namespace: object.namespace,
			Name:      object.name,
		},
		Kind: object.kind,
	}
}

func itoa(value int) string {
	return strconv.Itoa(value)
}
