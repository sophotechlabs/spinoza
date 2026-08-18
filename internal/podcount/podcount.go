package podcount

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/metadata"
)

const (
	probeLimit = 1
	pageSize   = 500
	maxPages   = 20
)

var podsGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}

type Result struct {
	Total    int
	Complete bool
}

func Count(ctx context.Context, client metadata.Interface, selector string) (Result, error) {
	probe, err := client.Resource(podsGVR).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		Limit:         probeLimit,
		FieldSelector: selector,
	})
	if err != nil {
		return Result{}, err
	}
	remaining := probe.GetRemainingItemCount()
	if remaining != nil {
		return Result{Total: len(probe.Items) + int(clamp(*remaining)), Complete: true}, nil
	}
	if probe.GetContinue() == "" {
		return Result{Total: len(probe.Items), Complete: true}, nil
	}
	return walk(ctx, client, selector)
}

func walk(ctx context.Context, client metadata.Interface, selector string) (Result, error) {
	total := 0
	opts := metav1.ListOptions{Limit: pageSize, FieldSelector: selector}
	for page := range maxPages {
		list, err := client.Resource(podsGVR).Namespace(metav1.NamespaceAll).List(ctx, opts)
		if err != nil {
			return Result{}, err
		}
		total += len(list.Items)
		if list.GetContinue() == "" {
			return Result{Total: total, Complete: true}, nil
		}
		if page == maxPages-1 {
			return Result{Total: total, Complete: false}, nil
		}
		opts.Continue = list.GetContinue()
	}
	return Result{Total: total, Complete: false}, nil
}

func clamp(remaining int64) int64 {
	if remaining < 0 {
		return 0
	}
	return remaining
}

func Limit() int {
	return pageSize * maxPages
}
