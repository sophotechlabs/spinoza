package checks

import (
	"slices"
	"strconv"
	"strings"
)

const (
	weightLow    = 0
	weightMedium = 1
	weightHigh   = 2
)

const spreadReplicas = 2

func baseWeight(severity string) int {
	switch severity {
	case severityHigh:
		return weightHigh
	case severityMedium:
		return weightMedium
	default:
		return weightLow
	}
}

func severityAt(weight int) string {
	switch {
	case weight >= weightHigh:
		return severityHigh
	case weight == weightMedium:
		return severityMedium
	default:
		return severityLow
	}
}

func blastWeight(base int, subject Subject) int {
	weight := base
	switch subject.Origin {
	case originSystem:
		weight -= 2
	case originPackaged:
		weight--
	default:
	}
	if subject.Replicas >= spreadReplicas {
		weight++
	}
	if weight < weightLow {
		return weightLow
	}
	if weight > weightHigh {
		return weightHigh
	}
	return weight
}

func severityFor(base string, subject Subject) string {
	return severityAt(blastWeight(baseWeight(base), subject))
}

func rankOf(severity string) string {
	return strconv.Itoa(weightHigh - baseWeight(severity))
}

func (c check) ranked(all []found) []found {
	for at := range all {
		all[at].severity = severityFor(c.severity, all[at].subject)
	}
	slices.SortStableFunc(all, func(left, right found) int {
		return strings.Compare(findingKey(left), findingKey(right))
	})
	return all
}
