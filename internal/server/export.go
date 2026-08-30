package server

import (
	"encoding/csv"
	"net/http"
	"strconv"

	"github.com/sophotechlabs/spinoza/internal/api"
)

// exportChecks writes the audit as CSV, one row per finding, under whatever
// filter the view is showing. The report a browser holds stops at the findings
// it displays, so this is built from a fresh audit rather than from the page.
func (s *Server) exportChecks(w http.ResponseWriter, r *http.Request) {
	report := s.managerFor(r).CheckExport(r.Context(), s.checkFilter(r))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="spinoza-checks.csv"`)
	out := csv.NewWriter(w)
	_ = out.Write([]string{
		"check", "title", "category", "severity", "kind", "namespace", "name",
		"container", "detail", "new", "muted", "reason",
	})
	for _, group := range report.Groups {
		writeGroup(out, report, group)
	}
	out.Flush()
}

func writeGroup(out *csv.Writer, report api.CheckReport, group api.CheckGroup) {
	for _, finding := range group.Findings {
		object := api.CheckObject{}
		if finding.Ref >= 0 && finding.Ref < len(report.Objects) {
			object = report.Objects[finding.Ref]
		}
		_ = out.Write([]string{
			group.ID, group.Title, group.Category, group.Severity,
			object.Kind, object.Namespace, object.Name,
			finding.Container, finding.Detail,
			strconv.FormatBool(finding.New), strconv.FormatBool(finding.Muted), finding.Reason,
		})
	}
}
