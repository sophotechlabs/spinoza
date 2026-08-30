package server

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const fleetImageCap = 500

func (s *Server) fleetInventory(w http.ResponseWriter, r *http.Request) {
	found := eachCluster(r.Context(), s, func(ctx context.Context, backend Backend) api.ResourceCounts {
		return backend.Counts(ctx)
	})
	writeJSON(w, mergeCounts(found))
}

// The inventory answers "how much of this do we have, and where", so a kind
// keeps its per-cluster split rather than collapsing to one number.
func mergeCounts(found []clusterAnswer[api.ResourceCounts]) api.FleetInventory {
	merged := api.FleetInventory{Kinds: []api.FleetKind{}}
	at := map[string]int{}
	trouble := []string{}
	for _, one := range found {
		for key, count := range one.answer.Counts {
			where, seen := at[key]
			if !seen {
				at[key] = len(merged.Kinds)
				merged.Kinds = append(merged.Kinds, api.FleetKind{Key: key, PerCluster: map[string]int{}})
				where = at[key]
			}
			into := &merged.Kinds[where]
			into.Total += count
			into.Failing += one.answer.Failing[key]
			into.PerCluster[one.cluster] = count
		}
		for what, why := range one.answer.Errors {
			trouble = append(trouble, one.context+" "+what+": "+why)
		}
	}
	slices.SortStableFunc(merged.Kinds, func(left, right api.FleetKind) int {
		if left.Total != right.Total {
			return right.Total - left.Total
		}
		return strings.Compare(left.Key, right.Key)
	})
	slices.Sort(trouble)
	merged.Error = strings.Join(trouble, " · ")
	return merged
}

var podsEverywhere = api.ObjectRef{Version: "v1", Resource: "pods"}

func (s *Server) fleetImages(w http.ResponseWriter, r *http.Request) {
	found := eachCluster(r.Context(), s, func(ctx context.Context, backend Backend) imageAnswer {
		held, err := backend.ListKind(ctx, podsEverywhere)
		if err != nil {
			return imageAnswer{err: err.Error()}
		}
		return imageAnswer{pods: held}
	})
	writeJSON(w, mergeImages(found))
}

type imageAnswer struct {
	pods []*unstructured.Unstructured
	err  string
}

// Two clusters running two tags of one repo is the drift a fleet has and a
// single cluster cannot see, so the repo carries the tags found beside it.
func mergeImages(found []clusterAnswer[imageAnswer]) api.FleetImages {
	held := map[string]*api.FleetImage{}
	byRepo := map[string]map[string]struct{}{}
	trouble := []string{}
	for _, one := range found {
		for _, pod := range one.answer.pods {
			countImages(held, byRepo, imagesIn(pod), one.cluster)
		}
		if one.answer.err != "" {
			trouble = append(trouble, one.context+": "+one.answer.err)
		}
	}
	out := make([]api.FleetImage, 0, len(held))
	for _, one := range held {
		slices.Sort(one.Clusters)
		one.Skew = skewOf(byRepo[one.Repo])
		out = append(out, *one)
	}
	slices.SortStableFunc(out, byUse)
	if len(out) > fleetImageCap {
		out = out[:fleetImageCap]
	}
	slices.Sort(trouble)
	return api.FleetImages{Images: out, Error: strings.Join(trouble, " · ")}
}

func countImages(
	held map[string]*api.FleetImage, byRepo map[string]map[string]struct{},
	images []string, cluster string,
) {
	for _, image := range images {
		repo, tag := splitImage(image)
		one, seen := held[image]
		if !seen {
			one = &api.FleetImage{Image: image, Repo: repo, Tag: tag, Clusters: []string{}}
			held[image] = one
		}
		one.Pods++
		if !slices.Contains(one.Clusters, cluster) {
			one.Clusters = append(one.Clusters, cluster)
		}
		if byRepo[repo] == nil {
			byRepo[repo] = map[string]struct{}{}
		}
		byRepo[repo][tag] = struct{}{}
	}
}

func skewOf(tags map[string]struct{}) []string {
	if len(tags) < 2 {
		return nil
	}
	out := make([]string, 0, len(tags))
	for tag := range tags {
		out = append(out, tag)
	}
	slices.Sort(out)
	return out
}

func byUse(left, right api.FleetImage) int {
	if len(left.Clusters) != len(right.Clusters) {
		return len(right.Clusters) - len(left.Clusters)
	}
	if left.Pods != right.Pods {
		return right.Pods - left.Pods
	}
	return strings.Compare(left.Image, right.Image)
}

// A digest pins the whole reference, so it stays part of the repo rather than
// being read as a tag nobody else has.
func splitImage(image string) (repo, tag string) {
	if held, digest, found := strings.Cut(image, "@"); found {
		return held, digest
	}
	at := strings.LastIndex(image, ":")
	if at < 0 {
		return image, ""
	}
	if strings.Contains(image[at:], "/") {
		return image, ""
	}
	return image[:at], image[at+1:]
}

func imagesIn(pod *unstructured.Unstructured) []string {
	out := []string{}
	for _, field := range []string{"containers", "initContainers"} {
		held, found, err := unstructured.NestedSlice(pod.Object, "spec", field)
		if err != nil || !found {
			continue
		}
		out = append(out, imagesOf(held)...)
	}
	return out
}

func imagesOf(held []any) []string {
	out := []string{}
	for _, entry := range held {
		one, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		image, ok := one["image"].(string)
		if !ok || image == "" {
			continue
		}
		out = append(out, image)
	}
	return out
}
