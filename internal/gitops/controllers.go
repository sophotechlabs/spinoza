package gitops

import (
	"context"
	"slices"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const partOfLabel = "app.kubernetes.io/part-of"

var controllerOwners = map[string]string{
	"flux":   api.ControllerFlux,
	"argocd": api.ControllerArgo,
}

func Controllers(ctx context.Context, cs kubernetes.Interface) []api.GitopsController {
	if cs == nil {
		return nil
	}
	found := []api.GitopsController{}
	for label, controller := range controllerOwners {
		found = append(found, running(ctx, cs, partOfLabel+"="+label, controller)...)
	}
	if len(found) == 0 {
		return nil
	}
	slices.SortFunc(found, func(left, right api.GitopsController) int {
		if left.Controller != right.Controller {
			return strings.Compare(left.Controller, right.Controller)
		}
		return strings.Compare(left.Name, right.Name)
	})
	return found
}

func running(ctx context.Context, cs kubernetes.Interface, selector, controller string) []api.GitopsController {
	out := []api.GitopsController{}
	options := metav1.ListOptions{LabelSelector: selector}
	deployments, err := cs.AppsV1().Deployments(metav1.NamespaceAll).List(ctx, options)
	if err == nil {
		for i := range deployments.Items {
			item := &deployments.Items[i]
			out = append(out, controllerOf(controller, item.Namespace, item.Name,
				item.Status.ReadyReplicas, item.Spec.Replicas))
		}
	}
	sets, setsErr := cs.AppsV1().StatefulSets(metav1.NamespaceAll).List(ctx, options)
	if setsErr == nil {
		for i := range sets.Items {
			item := &sets.Items[i]
			out = append(out, controllerOf(controller, item.Namespace, item.Name,
				item.Status.ReadyReplicas, item.Spec.Replicas))
		}
	}
	return out
}

func controllerOf(controller, namespace, name string, ready int32, replicas *int32) api.GitopsController {
	return api.GitopsController{
		Controller: controller,
		Name:       name,
		Namespace:  namespace,
		Ready:      int(ready),
		Wanted:     wanted(replicas),
	}
}

func wanted(replicas *int32) int {
	if replicas == nil {
		return 1
	}
	return int(*replicas)
}
