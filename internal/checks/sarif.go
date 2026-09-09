package checks

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/version"
)

const (
	sarifSchemaURI   = "https://json.schemastore.org/sarif-2.1.0.json"
	sarifVersion     = "2.1.0"
	sarifToolName    = "spinoza"
	sarifToolURI     = "https://spinoza.tech"
	sarifPrintKey    = "spinozaCheckIdentity/v1"
	sarifMuteKind    = "external"
	sarifNoObject    = "an object this report did not carry"
	sarifIdentitySep = "\x00"
)

type sarifLevels struct {
	level   string
	problem string
}

var sarifBySeverity = map[string]sarifLevels{
	severityHigh:   {level: "error", problem: "error"},
	severityMedium: {level: "warning", problem: "warning"},
	severityLow:    {level: "note", problem: "recommendation"},
}

var sarifUnknownSeverity = sarifLevels{level: "none", problem: "recommendation"}

type SARIFLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []SARIFRun `json:"runs"`
}

type SARIFRun struct {
	Tool    SARIFTool     `json:"tool"`
	Results []SARIFResult `json:"results"`
}

type SARIFTool struct {
	Driver SARIFDriver `json:"driver"`
}

type SARIFDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version"`
	InformationURI string      `json:"informationUri"`
	Rules          []SARIFRule `json:"rules"`
}

type SARIFRule struct {
	ID                   string              `json:"id"`
	Name                 string              `json:"name"`
	ShortDescription     SARIFText           `json:"shortDescription"`
	FullDescription      SARIFText           `json:"fullDescription"`
	Help                 SARIFText           `json:"help"`
	DefaultConfiguration SARIFConfiguration  `json:"defaultConfiguration"`
	Properties           SARIFRuleProperties `json:"properties"`
}

type SARIFConfiguration struct {
	Level string `json:"level"`
}

type SARIFRuleProperties struct {
	Tags    []string     `json:"tags"`
	Problem SARIFProblem `json:"problem"`
}

type SARIFProblem struct {
	Severity string `json:"severity"`
}

type SARIFText struct {
	Text string `json:"text"`
}

type SARIFResult struct {
	RuleID              string             `json:"ruleId"`
	Level               string             `json:"level"`
	Message             SARIFText          `json:"message"`
	Locations           []SARIFLocation    `json:"locations"`
	PartialFingerprints map[string]string  `json:"partialFingerprints"`
	Suppressions        []SARIFSuppression `json:"suppressions,omitempty"`
}

type SARIFLocation struct {
	LogicalLocations []SARIFLogicalLocation `json:"logicalLocations"`
}

type SARIFLogicalLocation struct {
	Name               string `json:"name"`
	FullyQualifiedName string `json:"fullyQualifiedName"`
}

type SARIFSuppression struct {
	Kind          string `json:"kind"`
	Justification string `json:"justification,omitempty"`
}

func SARIF(report api.CheckReport) SARIFLog {
	return SARIFLog{
		Schema:  sarifSchemaURI,
		Version: sarifVersion,
		Runs: []SARIFRun{{
			Tool: SARIFTool{Driver: SARIFDriver{
				Name:           sarifToolName,
				Version:        version.String(),
				InformationURI: sarifToolURI,
				Rules:          sarifRules(report),
			}},
			Results: sarifResults(report),
		}},
	}
}

func sarifLevelsFor(severity string) sarifLevels {
	levels, known := sarifBySeverity[severity]
	if !known {
		return sarifUnknownSeverity
	}
	return levels
}

func sarifRules(report api.CheckReport) []SARIFRule {
	out := make([]SARIFRule, 0, len(report.Groups))
	for _, group := range report.Groups {
		levels := sarifLevelsFor(group.Severity)
		out = append(out, SARIFRule{
			ID:                   group.ID,
			Name:                 group.ID,
			ShortDescription:     SARIFText{Text: group.Title},
			FullDescription:      SARIFText{Text: group.Wrong},
			Help:                 SARIFText{Text: group.Remedy},
			DefaultConfiguration: SARIFConfiguration{Level: levels.level},
			Properties: SARIFRuleProperties{
				Tags:    sarifTags(group),
				Problem: SARIFProblem{Severity: levels.problem},
			},
		})
	}
	return out
}

func sarifTags(group api.CheckGroup) []string {
	out := make([]string, 0, len(group.Frameworks)+1)
	if group.Category != "" {
		out = append(out, group.Category)
	}
	return append(out, group.Frameworks...)
}

func sarifResults(report api.CheckReport) []SARIFResult {
	out := make([]SARIFResult, 0, sarifFindings(report))
	for _, group := range report.Groups {
		for _, finding := range group.Findings {
			out = append(out, sarifResultFor(report, group, finding))
		}
	}
	return out
}

func sarifFindings(report api.CheckReport) int {
	total := 0
	for _, group := range report.Groups {
		total += len(group.Findings)
	}
	return total
}

func sarifResultFor(report api.CheckReport, group api.CheckGroup, finding api.CheckFinding) SARIFResult {
	object := sarifObjectOf(report, finding)
	return SARIFResult{
		RuleID:              group.ID,
		Level:               sarifLevelsFor(sarifSeverityOf(group, finding)).level,
		Message:             SARIFText{Text: sarifMessage(group, object, finding)},
		Locations:           sarifLocations(object),
		PartialFingerprints: map[string]string{sarifPrintKey: sarifFingerprint(group.ID, object, finding)},
		Suppressions:        sarifSuppressions(finding),
	}
}

func sarifObjectOf(report api.CheckReport, finding api.CheckFinding) api.CheckObject {
	if finding.Ref < 0 || finding.Ref >= len(report.Objects) {
		return api.CheckObject{}
	}
	return report.Objects[finding.Ref]
}

func sarifSeverityOf(group api.CheckGroup, finding api.CheckFinding) string {
	if finding.Severity != "" {
		return finding.Severity
	}
	return group.Severity
}

func sarifLocations(object api.CheckObject) []SARIFLocation {
	path := sarifPath(object)
	if path == "" {
		return []SARIFLocation{}
	}
	return []SARIFLocation{{
		LogicalLocations: []SARIFLogicalLocation{{
			Name:               object.Name,
			FullyQualifiedName: path,
		}},
	}}
}

func sarifPath(object api.CheckObject) string {
	parts := make([]string, 0, 3)
	if object.Kind != "" {
		parts = append(parts, object.Kind)
	}
	if object.Namespace != "" {
		parts = append(parts, object.Namespace)
	}
	if object.Name != "" {
		parts = append(parts, object.Name)
	}
	return strings.Join(parts, "/")
}

func sarifMessage(group api.CheckGroup, object api.CheckObject, finding api.CheckFinding) string {
	subject := sarifSubject(object)
	if finding.Container != "" {
		subject += " container " + finding.Container
	}
	return subject + ": " + sarifWrong(group, finding)
}

func sarifSubject(object api.CheckObject) string {
	name := object.Name
	if object.Namespace != "" && object.Name != "" {
		name = object.Namespace + "/" + object.Name
	}
	if object.Kind == "" && name == "" {
		return sarifNoObject
	}
	if object.Kind == "" {
		return name
	}
	if name == "" {
		return object.Kind
	}
	return object.Kind + " " + name
}

func sarifWrong(group api.CheckGroup, finding api.CheckFinding) string {
	if finding.Detail != "" {
		return finding.Detail
	}
	if group.Wrong != "" {
		return group.Wrong
	}
	return group.Title
}

func sarifFingerprint(id string, object api.CheckObject, finding api.CheckFinding) string {
	identity := strings.Join([]string{
		id, object.Kind, object.Namespace, object.Name, finding.Container,
	}, sarifIdentitySep)
	sum := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(sum[:])
}

func sarifSuppressions(finding api.CheckFinding) []SARIFSuppression {
	if !finding.Muted {
		return nil
	}
	return []SARIFSuppression{{Kind: sarifMuteKind, Justification: finding.Reason}}
}
