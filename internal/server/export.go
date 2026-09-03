package server

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"unicode"
	"unicode/utf8"

	"github.com/sophotechlabs/spinoza/internal/api"
)

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
		_ = out.Write(spreadsheetCells([]string{
			group.ID, group.Title, group.Category, group.Severity,
			object.Kind, object.Namespace, object.Name,
			finding.Container, finding.Detail,
			strconv.FormatBool(finding.New), strconv.FormatBool(finding.Muted), finding.Reason,
		}))
	}
}

func spreadsheetCells(cells []string) []string {
	for at, value := range cells {
		if spreadsheetFormula(value) {
			cells[at] = "'" + value
		}
	}
	return cells
}

func spreadsheetFormula(value string) bool {
	for value != "" {
		first, size := utf8.DecodeRuneInString(value)
		switch first {
		case '=', '+', '-', '@', 0, '＝', '＋', '－', '＠':
			return true
		}
		if unicode.IsControl(first) {
			return true
		}
		if !unicode.IsSpace(first) {
			return false
		}
		value = value[size:]
	}
	return false
}
