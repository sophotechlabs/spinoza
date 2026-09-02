package podcount

import (
	"context"
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/metadata"
)

var errRepeatedContinue = errors.New("the apiserver repeated a continuation token while counting pods")

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
		Limit:         probeSize(selector),
		FieldSelector: selector,
	})
	if err != nil {
		return Result{}, err
	}
	if probe.GetContinue() == "" {
		return Result{Total: len(probe.Items), Complete: true}, nil
	}
	return walk(ctx, client, selector, len(probe.Items), probe.GetContinue())
}

func probeSize(selector string) int64 {
	if selector == "" {
		return probeLimit
	}
	return pageSize
}

func walk(
	ctx context.Context,
	client metadata.Interface,
	selector string,
	counted int,
	from string,
) (Result, error) {
	total := counted
	seen := map[string]bool{from: true}
	opts := metav1.ListOptions{Limit: pageSize, FieldSelector: selector, Continue: from}
	for range maxPages {
		list, err := client.Resource(podsGVR).Namespace(metav1.NamespaceAll).List(ctx, opts)
		if err != nil {
			return Result{}, err
		}
		total += len(list.Items)
		next := list.GetContinue()
		if next == "" {
			return Result{Total: total, Complete: true}, nil
		}
		if seen[next] {
			return Result{}, errRepeatedContinue
		}
		seen[next] = true
		opts.Continue = next
	}
	if total > Limit() {
		total = Limit()
	}
	return Result{Total: total, Complete: false}, nil
}

func Limit() int {
	return pageSize * maxPages
}
