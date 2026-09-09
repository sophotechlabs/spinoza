package rollout

import (
	"context"
	"errors"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/compare"
)

func Diff(ctx context.Context, src Source, ref api.ObjectRef, from, to int64) (api.RevisionDiff, error) {
	found, reason, err := history(ctx, src, ref)
	if err != nil {
		return api.RevisionDiff{}, err
	}
	if reason != "" {
		return api.RevisionDiff{}, errors.New(reason)
	}
	left, leftErr := rendered(found, ref, from)
	if leftErr != nil {
		return api.RevisionDiff{}, leftErr
	}
	right, rightErr := rendered(found, ref, to)
	if rightErr != nil {
		return api.RevisionDiff{}, rightErr
	}
	return api.RevisionDiff{
		From:  from,
		To:    to,
		Lines: differingLines(left, right),
		Same:  left == right,
		Left:  left,
		Right: right,
	}, nil
}

func rendered(found []entry, ref api.ObjectRef, number int64) (string, error) {
	template, err := pick(found, ref, number)
	if err != nil {
		return "", err
	}
	return compare.YAML(compare.Normalise(&unstructured.Unstructured{Object: template}))
}

func differingLines(left, right string) int {
	counted := map[string]int{}
	for line := range strings.SplitSeq(left, "\n") {
		counted[line]++
	}
	total := 0
	for line := range strings.SplitSeq(right, "\n") {
		if counted[line] > 0 {
			counted[line]--
			continue
		}
		total++
	}
	for _, count := range counted {
		total += count
	}
	return total
}
