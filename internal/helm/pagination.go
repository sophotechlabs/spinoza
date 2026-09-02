package helm

import (
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var errRepeatedContinue = errors.New("the apiserver repeated a continuation token")

func advancePage(opts *metav1.ListOptions, next string, seen map[string]bool) (bool, error) {
	if next == "" {
		return false, nil
	}
	if seen[next] {
		return false, errRepeatedContinue
	}
	seen[next] = true
	opts.Continue = next
	return true, nil
}
