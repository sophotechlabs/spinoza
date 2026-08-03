package gitops

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type dynLister struct {
	dyn dynamic.Interface
}

func listerFor(dyn dynamic.Interface) *dynLister {
	return &dynLister{dyn: dyn}
}

func (d *dynLister) List(desc api.ResourceDescriptor) ([]*unstructured.Unstructured, error) {
	gvr := schema.GroupVersionResource{Group: desc.Group, Version: desc.Version, Resource: desc.Resource}
	list, err := d.dyn.Resource(gvr).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]*unstructured.Unstructured, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, &list.Items[i])
	}
	return out, nil
}
