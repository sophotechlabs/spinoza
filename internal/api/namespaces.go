package api

import "strings"

func WorstNamespaceFirst(left, right NamespaceCount) int {
	if left.High != right.High {
		return right.High - left.High
	}
	if left.Total != right.Total {
		return right.Total - left.Total
	}
	return strings.Compare(left.Namespace, right.Namespace)
}
