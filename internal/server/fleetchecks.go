package server

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type standDown struct {
	cluster string
	group   string
	title   string
	why     string
}

type standDownKey struct {
	cluster string
	why     string
}

func (s *Server) fleetChecks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, mergeReports(s.everyClustersChecks(r.Context(), r)))
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
	ran := map[string]bool{}
	stood := make([]standDown, 0, len(found))
	taken := []string{}
	unbaselined := []string{}
	for _, one := range found {
		offset := len(merged.Objects)
		merged.Objects = append(merged.Objects, stamped(one)...)
		foldGroups(&merged, at, one.answer.Groups, offset)
		stood = append(stood, standDownsIn(one, ran)...)
		foldNamespaces(spread, one.answer.Namespaces)
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
	merged.Namespaces = spreadOf(spread)
	merged.Baseline = earliest(taken)
	slices.Sort(trouble)
	merged.Error = strings.Join(append(trouble, notes(stood, ran, taken, unbaselined)...), " · ")
	return merged
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

func standDownsIn(one clusterAnswer[api.CheckReport], ran map[string]bool) []standDown {
	out := []standDown{}
	for _, group := range one.answer.Groups {
		if group.Skipped == "" {
			ran[group.ID] = true
			continue
		}
		out = append(out, standDown{
			cluster: one.context,
			group:   group.ID,
			title:   group.Title,
			why:     group.Skipped,
		})
	}
	return out
}

func notes(stood []standDown, ran map[string]bool, taken, unbaselined []string) []string {
	out := standDownNotes(stood, ran)
	if len(taken) > 0 {
		out = append(out, baselineNotes(unbaselined)...)
	}
	slices.Sort(out)
	return out
}

func standDownNotes(stood []standDown, ran map[string]bool) []string {
	titles := map[standDownKey][]string{}
	for _, one := range stood {
		if !ran[one.group] {
			continue
		}
		key := standDownKey{cluster: one.cluster, why: one.why}
		titles[key] = append(titles[key], one.title)
	}
	out := make([]string, 0, len(titles))
	for key, held := range titles {
		slices.Sort(held)
		out = append(out, key.cluster+": "+checkWord(len(held))+
			" did not run there ("+key.why+"): "+strings.Join(held, ", "))
	}
	return out
}

func checkWord(count int) string {
	if count == 1 {
		return "1 check"
	}
	return strconv.Itoa(count) + " checks"
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

func foldNamespaces(spread map[string]api.NamespaceCount, counts []api.NamespaceCount) {
	for _, count := range counts {
		into := spread[count.Namespace]
		into.Namespace = count.Namespace
		into.Total += count.Total
		into.High += count.High
		into.Medium += count.Medium
		into.Low += count.Low
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

func foldGroups(merged *api.CheckReport, at map[string]int, groups []api.CheckGroup, offset int) {
	for _, group := range groups {
		found := shifted(group, offset)
		where, seen := at[group.ID]
		if !seen {
			at[group.ID] = len(merged.Groups)
			merged.Groups = append(merged.Groups, found)
			continue
		}
		into := &merged.Groups[where]
		if found.Skipped != "" {
			continue
		}
		if into.Skipped != "" {
			*into = found
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
		out = append(out, count)
	}
	slices.SortFunc(out, func(left, right api.NamespaceCount) int {
		if left.Total != right.Total {
			return right.Total - left.Total
		}
		return strings.Compare(left.Namespace, right.Namespace)
	})
	return out
}
