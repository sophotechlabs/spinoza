package checks

import (
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"

	"k8s.io/client-go/kubernetes/scheme"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type Facts struct {
	ServerVersion  string
	ServedVersions []string
	Warnings       []string
}

type removal struct {
	groupVersion string
	kinds        string
	minor        int
}

type removalKey struct {
	groupVersion string
	minor        int
}

type removedAPI interface {
	APILifecycleRemoved() (int, int)
}

var removals = sync.OnceValue(collectRemovals)

func collectRemovals() []removal {
	grouped := map[removalKey][]string{}
	for gvk, typ := range scheme.Scheme.AllKnownTypes() {
		if strings.HasSuffix(gvk.Kind, "List") {
			continue
		}
		marked, ok := reflect.TypeAssert[removedAPI](reflect.New(typ))
		if !ok {
			continue
		}
		major, minor := marked.APILifecycleRemoved()
		if major != 1 {
			continue
		}
		key := removalKey{groupVersion: gvk.GroupVersion().String(), minor: minor}
		grouped[key] = append(grouped[key], gvk.Kind)
	}
	return sortedRemovals(grouped)
}

func sortedRemovals(grouped map[removalKey][]string) []removal {
	out := make([]removal, 0, len(grouped))
	for key, kinds := range grouped {
		slices.Sort(kinds)
		out = append(out, removal{
			groupVersion: key.groupVersion,
			kinds:        strings.Join(kinds, ", "),
			minor:        key.minor,
		})
	}
	slices.SortFunc(out, func(one, two removal) int {
		if one.groupVersion != two.groupVersion {
			return strings.Compare(one.groupVersion, two.groupVersion)
		}
		return one.minor - two.minor
	})
	return out
}

func deprecationChecks() []check {
	return []check{
		{
			id:       "serves-a-removed-api",
			title:    "Cluster still serves an API a release removes",
			category: categoryReliability,
			severity: severityHigh,
			wrong:    "Anything still writing to this version stops working the moment the cluster is upgraded past the release that removes it.",
			remedy:   "Move whatever uses it to the replacement version before that upgrade.",
			find:     overCorpus(servesARemovedAPI),
		},
		{
			id:       "apiserver-says-deprecated",
			title:    "The apiserver warned about something the audit asked for",
			category: categoryReliability,
			severity: severityLow,
			wrong:    "Your own cluster answered a request with a deprecation warning. This is what it has said since Spinoza connected, so a window open for an hour has seen more than one just started.",
			remedy:   "Move to what the warning names.",
			find:     overCorpus(apiserverSaysDeprecated),
		},
	}
}

func minorOf(version string) int {
	trimmed := strings.TrimPrefix(version, "v")
	parts := strings.Split(trimmed, ".")
	if len(parts) < 2 {
		return 0
	}
	minor, err := strconv.Atoi(strings.TrimSuffix(parts[1], "+"))
	if err != nil {
		return 0
	}
	return minor
}

func servesARemovedAPI(sc scan) []found {
	running := minorOf(sc.facts.ServerVersion)
	if running == 0 {
		return nil
	}
	out := []found{}
	for _, one := range removals() {
		if one.minor <= running {
			continue
		}
		if !slices.Contains(sc.facts.ServedVersions, one.groupVersion) {
			continue
		}
		out = append(out, clusterFinding("serves "+one.groupVersion,
			one.groupVersion+" carries "+one.kinds+" and is removed in 1."+
				strconv.Itoa(one.minor)+"; this cluster is 1."+strconv.Itoa(running)))
	}
	return out
}

func apiserverSaysDeprecated(sc scan) []found {
	out := make([]found, 0, len(sc.facts.Warnings))
	for _, text := range sc.facts.Warnings {
		out = append(out, clusterFinding(warningSubject(text), text))
	}
	return out
}

func warningSubject(text string) string {
	head, _, found := strings.Cut(text, " is deprecated")
	if !found {
		return "the apiserver"
	}
	return head
}

func clusterFinding(name, detail string) found {
	return found{
		subject: Subject{
			Ref:  api.ObjectRef{Version: "v1", Resource: "clusters", Name: name},
			Kind: "Cluster",
		},
		detail: detail,
	}
}
