package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func (s *Server) fleetChecks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, mergeReports(s.everyClustersChecks(r.Context(), r)))
}

type fleetCheckCursor struct {
	Check string            `json:"check"`
	After map[string]string `json:"after"`
}

type fleetCheckPageResult struct {
	page api.CheckPage
	err  error
}

var errFleetChecksChanged = errors.New("the fleet changed")

func (s *Server) fleetCheckPage(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	id := query.Get("check")
	if id == "" {
		writeError(w, http.StatusBadRequest, "check is required")
		return
	}
	cursor, err := decodeFleetCheckCursor(query.Get("after"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if cursor.Check != id {
		writeError(w, http.StatusBadRequest, "the fleet findings cursor belongs to another check")
		return
	}
	if missing := s.missingFleetCursorCluster(cursor); missing != "" {
		writeError(w, http.StatusConflict, missing+" is no longer open; refresh the fleet checks")
		return
	}
	found := eachOpenCluster(r.Context(), s,
		func(ctx context.Context, one api.OpenCluster, backend Backend) fleetCheckPageResult {
			after, wanted := cursor.After[one.ID]
			if !wanted {
				return fleetCheckPageResult{}
			}
			page, pageErr := backend.CheckPage(ctx, id, after, s.checkFilterOn(r, one.ID))
			return fleetCheckPageResult{page: page, err: pageErr}
		})
	page, err := mergeFleetCheckPages(id, cursor.After, found)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, errFleetChecksChanged) {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, page)
}

func (s *Server) missingFleetCursorCluster(cursor fleetCheckCursor) string {
	open := map[string]bool{}
	for _, one := range s.cluster.Opened() {
		open[one.ID] = s.managerOf(one.ID) != nil
	}
	for cluster := range cursor.After {
		if !open[cluster] {
			return cluster
		}
	}
	return ""
}

func (s *Server) everyClustersChecks(ctx context.Context, r *http.Request) []clusterAnswer[api.CheckReport] {
	return eachOpenCluster(ctx, s, func(ctx context.Context, one api.OpenCluster, backend Backend) api.CheckReport {
		return backend.Checks(ctx, s.checkFilterOn(r, one.ID))
	})
}

func mergeReports(found []clusterAnswer[api.CheckReport]) api.CheckReport {
	merged := api.CheckReport{Groups: []api.CheckGroup{}, Objects: []api.CheckObject{}}
	at := map[string]int{}
	trouble := []string{}
	spread := map[string]api.NamespaceCount{}
	taken := []string{}
	unbaselined := []string{}
	for _, one := range found {
		offset := len(merged.Objects)
		merged.Objects = append(merged.Objects, stamped(one)...)
		foldGroups(&merged, at, one.answer.Groups, offset, one.context)
		foldNamespaces(spread, one.answer.Namespaces, one.context)
		merged.Scanned += one.answer.Scanned
		merged.WasScanned += one.answer.WasScanned
		if one.answer.Baseline != "" {
			taken = append(taken, one.answer.Baseline)
		}
		if lacksBaseline(one) {
			unbaselined = append(unbaselined, one.context)
		}
		if one.answer.Error != "" {
			trouble = append(trouble, one.context+": "+one.answer.Error)
		}
		if one.failure != "" {
			trouble = append(trouble, one.context+": "+one.failure)
		}
	}
	settleStandDowns(merged.Groups)
	setFleetCheckCursors(merged.Groups, found)
	merged.Namespaces = spreadOf(spread)
	merged.Baseline = earliest(taken)
	slices.Sort(trouble)
	merged.Error = strings.Join(append(trouble, notes(taken, unbaselined)...), " · ")
	return merged
}

func setFleetCheckCursors(groups []api.CheckGroup, found []clusterAnswer[api.CheckReport]) {
	after := map[string]map[string]string{}
	for _, one := range found {
		if one.failure != "" {
			continue
		}
		for _, group := range one.answer.Groups {
			if group.Next == "" {
				continue
			}
			if after[group.ID] == nil {
				after[group.ID] = map[string]string{}
			}
			after[group.ID][one.cluster] = group.Next
		}
	}
	for at := range groups {
		if len(after[groups[at].ID]) == 0 {
			continue
		}
		groups[at].Next = encodeFleetCheckCursor(groups[at].ID, after[groups[at].ID])
	}
}

func encodeFleetCheckCursor(check string, after map[string]string) string {
	body, err := json.Marshal(fleetCheckCursor{Check: check, After: after})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(body)
}

func decodeFleetCheckCursor(raw string) (fleetCheckCursor, error) {
	if raw == "" {
		return fleetCheckCursor{}, errors.New("after is required")
	}
	body, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return fleetCheckCursor{}, errors.New("the fleet findings cursor is invalid")
	}
	var cursor fleetCheckCursor
	if err := json.Unmarshal(body, &cursor); err != nil {
		return fleetCheckCursor{}, errors.New("the fleet findings cursor is invalid")
	}
	if cursor.Check == "" || len(cursor.After) == 0 {
		return fleetCheckCursor{}, errors.New("the fleet findings cursor is invalid")
	}
	for cluster, after := range cursor.After {
		if cluster == "" || after == "" {
			return fleetCheckCursor{}, errors.New("the fleet findings cursor is invalid")
		}
	}
	return cursor, nil
}

func mergeFleetCheckPages(
	check string,
	after map[string]string,
	found []clusterAnswer[fleetCheckPageResult],
) (api.CheckPage, error) {
	merged := api.CheckPage{Findings: []api.CheckFinding{}, Objects: []api.CheckObject{}}
	next := map[string]string{}
	answered := map[string]bool{}
	for _, one := range found {
		if _, wanted := after[one.cluster]; !wanted {
			continue
		}
		answered[one.cluster] = true
		if one.failure != "" {
			return api.CheckPage{}, fmt.Errorf("%s: %s", one.context, one.failure)
		}
		if one.answer.err != nil {
			return api.CheckPage{}, fmt.Errorf("%s: %w", one.context, one.answer.err)
		}
		offset := len(merged.Objects)
		for _, object := range one.answer.page.Objects {
			object.Cluster = one.cluster
			merged.Objects = append(merged.Objects, object)
		}
		for _, finding := range one.answer.page.Findings {
			finding.Ref += offset
			merged.Findings = append(merged.Findings, finding)
		}
		if one.answer.page.Next != "" {
			next[one.cluster] = one.answer.page.Next
		}
	}
	missing := []string{}
	for cluster := range after {
		if !answered[cluster] {
			missing = append(missing, cluster)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		return api.CheckPage{}, fmt.Errorf(
			"%w: %s is no longer open; refresh the fleet checks",
			errFleetChecksChanged,
			missing[0],
		)
	}
	if len(next) > 0 {
		merged.Next = encodeFleetCheckCursor(check, next)
	}
	return merged, nil
}

func settleStandDowns(groups []api.CheckGroup) {
	for at := range groups {
		if groups[at].Skipped != "" {
			groups[at].PartialOn = nil
			continue
		}
		slices.Sort(groups[at].PartialOn)
	}
}

func lacksBaseline(one clusterAnswer[api.CheckReport]) bool {
	if one.answer.Baseline != "" {
		return false
	}
	if one.failure != "" {
		return false
	}
	return one.answer.Error == ""
}

func notes(taken, unbaselined []string) []string {
	out := []string{}
	if len(taken) > 0 {
		out = append(out, baselineNotes(unbaselined)...)
	}
	slices.Sort(out)
	return out
}

func baselineNotes(unbaselined []string) []string {
	out := make([]string, 0, len(unbaselined))
	for _, cluster := range unbaselined {
		out = append(out, cluster+": no baseline taken there, so nothing from it is marked new")
	}
	return out
}

func earliest(taken []string) string {
	if len(taken) == 0 {
		return ""
	}
	slices.Sort(taken)
	return taken[0]
}

func foldNamespaces(spread map[string]api.NamespaceCount, counts []api.NamespaceCount, cluster string) {
	for _, count := range counts {
		into := spread[count.Namespace]
		into.Namespace = count.Namespace
		into.Total += count.Total
		into.High += count.High
		into.Medium += count.Medium
		into.Low += count.Low
		if !slices.Contains(into.Clusters, cluster) {
			into.Clusters = append(into.Clusters, cluster)
		}
		spread[count.Namespace] = into
	}
}

func stamped(one clusterAnswer[api.CheckReport]) []api.CheckObject {
	out := make([]api.CheckObject, 0, len(one.answer.Objects))
	for _, obj := range one.answer.Objects {
		obj.Cluster = one.cluster
		out = append(out, obj)
	}
	return out
}

func foldGroups(
	merged *api.CheckReport,
	at map[string]int,
	groups []api.CheckGroup,
	offset int,
	on string,
) {
	for _, group := range groups {
		found := shifted(group, offset)
		where, seen := at[group.ID]
		if !seen {
			at[group.ID] = len(merged.Groups)
			if found.Skipped != "" {
				found.PartialOn = []string{stoodDownOn(on, found.Skipped)}
			}
			merged.Groups = append(merged.Groups, found)
			continue
		}
		into := &merged.Groups[where]
		if found.Skipped != "" {
			into.PartialOn = append(into.PartialOn, stoodDownOn(on, found.Skipped))
			continue
		}
		if into.Skipped != "" {
			held := into.PartialOn
			*into = found
			into.PartialOn = held
			continue
		}
		into.Total += found.Total
		into.Muted += found.Muted
		into.NewCount += found.NewCount
		into.Fixed += found.Fixed
		into.Was += found.Was
		into.Gone = append(into.Gone, found.Gone...)
		into.Baselined = into.Baselined && found.Baselined
		into.Ran = into.Ran && found.Ran
		into.Truncated = into.Truncated || found.Truncated
		into.Findings = append(into.Findings, found.Findings...)
	}
}

func stoodDownOn(cluster, why string) string {
	return cluster + ": " + why
}

func shifted(group api.CheckGroup, offset int) api.CheckGroup {
	moved := make([]api.CheckFinding, 0, len(group.Findings))
	for _, one := range group.Findings {
		one.Ref += offset
		moved = append(moved, one)
	}
	group.Findings = moved
	group.Next = ""
	return group
}

func spreadOf(counts map[string]api.NamespaceCount) []api.NamespaceCount {
	if len(counts) == 0 {
		return nil
	}
	out := make([]api.NamespaceCount, 0, len(counts))
	for _, count := range counts {
		slices.Sort(count.Clusters)
		out = append(out, count)
	}
	slices.SortFunc(out, api.WorstNamespaceFirst)
	return out
}
