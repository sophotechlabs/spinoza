package compare

import (
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func Kinds(left, right []*unstructured.Unstructured, byName bool) []api.KindDiff {
	here := index(left, byName)
	there := index(right, byName)

	out := make([]api.KindDiff, 0, len(here)+len(there))
	for key, object := range here {
		counterpart, paired := there[key]
		if !paired {
			out = append(out, verdictFor(object, api.VerdictOnlyHere, 0))
			continue
		}
		out = append(out, compared(object, counterpart))
	}
	for key, object := range there {
		_, paired := here[key]
		if paired {
			continue
		}
		out = append(out, verdictFor(object, api.VerdictOnlyThere, 0))
	}
	slices.SortFunc(out, byNamespaceThenName)
	return out
}

func compared(here, there *unstructured.Unstructured) api.KindDiff {
	left, leftErr := YAML(Normalise(here))
	right, rightErr := YAML(Normalise(there))
	if leftErr != nil || rightErr != nil {
		return verdictFor(here, api.VerdictDiffers, 0)
	}
	if left == right {
		return verdictFor(here, api.VerdictSame, 0)
	}
	return verdictFor(here, api.VerdictDiffers, changedLines(left, right))
}

func verdictFor(object *unstructured.Unstructured, verdict string, lines int) api.KindDiff {
	return api.KindDiff{
		Namespace: object.GetNamespace(),
		Name:      object.GetName(),
		Verdict:   verdict,
		Lines:     lines,
	}
}

func changedLines(left, right string) int {
	inRight := map[string]int{}
	for line := range strings.SplitSeq(right, "\n") {
		inRight[line]++
	}
	changed := 0
	for line := range strings.SplitSeq(left, "\n") {
		if inRight[line] > 0 {
			inRight[line]--
			continue
		}
		changed++
	}
	for _, count := range inRight {
		changed += count
	}
	return changed
}

func index(objects []*unstructured.Unstructured, byName bool) map[string]*unstructured.Unstructured {
	out := make(map[string]*unstructured.Unstructured, len(objects))
	for _, object := range objects {
		out[keyOf(object, byName)] = object
	}
	return out
}

func keyOf(object *unstructured.Unstructured, byName bool) string {
	if byName {
		return object.GetName()
	}
	return object.GetNamespace() + "/" + object.GetName()
}

func byNamespaceThenName(left, right api.KindDiff) int {
	if left.Namespace != right.Namespace {
		return strings.Compare(left.Namespace, right.Namespace)
	}
	return strings.Compare(left.Name, right.Name)
}

func Tally(objects []api.KindDiff) (same, differs, onlyHere, onlyThere int) {
	for _, object := range objects {
		switch object.Verdict {
		case api.VerdictSame:
			same++
		case api.VerdictDiffers:
			differs++
		case api.VerdictOnlyHere:
			onlyHere++
		case api.VerdictOnlyThere:
			onlyThere++
		default:
		}
	}
	return same, differs, onlyHere, onlyThere
}
