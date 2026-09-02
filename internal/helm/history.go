package helm

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const historyPageSize = 10

const maxHistoryRefs = maxObjects

var errAmbiguousRevision = errors.New("a helm revision is stored more than once")

func releaseSelector(name string) string {
	return ownerLabel + "," + nameLabel + "=" + name
}

func revisionSelector(name string, revisions []int64) string {
	values := make([]string, 0, len(revisions))
	for _, revision := range revisions {
		values = append(values, strconv.FormatInt(revision, 10))
	}
	selector := releaseSelector(name) + "," + versionLabel
	if len(values) == 1 {
		return selector + "=" + values[0]
	}
	return selector + " in (" + strings.Join(values, ",") + ")"
}

func (s *Service) storedRefs(
	ctx context.Context,
	namespace, selector string,
	limit int,
) ([]storedRef, error) {
	if s.meta == nil {
		return nil, errNoMetadata
	}
	secrets, err := listRefsLimited(
		ctx,
		s.meta,
		DriverSecret,
		secretsGVR,
		namespace,
		selector,
		limit+1,
	)
	if err != nil {
		return nil, err
	}
	maps, mapErr := listRefsLimited(
		ctx,
		s.meta,
		DriverConfigMap,
		configMapsGVR,
		namespace,
		selector,
		limit+1,
	)
	if mapErr != nil {
		return nil, mapErr
	}
	if secrets.truncated || maps.truncated {
		return nil, fmt.Errorf("more than %d stored revisions matched", limit)
	}
	refs := append([]storedRef{}, secrets.items...)
	refs = append(refs, maps.items...)
	if len(refs) > limit {
		return nil, fmt.Errorf("more than %d stored revisions matched", limit)
	}
	return refs, nil
}

func newerRef(left, right storedRef) int {
	if revision := cmp.Compare(right.revision, left.revision); revision != 0 {
		return revision
	}
	if driver := strings.Compare(left.driver, right.driver); driver != 0 {
		return driver
	}
	return strings.Compare(left.object, right.object)
}

func oneRef(refs []storedRef, namespace, name string) (storedRef, error) {
	if len(refs) == 0 {
		return storedRef{}, fmt.Errorf("%w: %s/%s", ErrNoRelease, namespace, name)
	}
	slices.SortFunc(refs, newerRef)
	if refs[0].revision < 1 {
		return storedRef{}, fmt.Errorf("%w: %s/%s", ErrNoRelease, namespace, name)
	}
	if len(refs) > 1 && refs[0].revision == refs[1].revision {
		return storedRef{}, fmt.Errorf("%w: revision %d", errAmbiguousRevision, refs[0].revision)
	}
	return refs[0], nil
}

func (s *Service) latestRef(ctx context.Context, namespace, name string) (storedRef, error) {
	refs, err := s.storedRefs(ctx, namespace, releaseSelector(name), maxHistoryRefs)
	if err != nil {
		return storedRef{}, err
	}
	return oneRef(refs, namespace, name)
}

func (s *Service) refAt(ctx context.Context, namespace, name string, revision int64) (storedRef, error) {
	refs, err := s.storedRefs(ctx, namespace, revisionSelector(name, []int64{revision}), 1)
	if err != nil {
		return storedRef{}, err
	}
	return oneRef(refs, namespace, name)
}

func selectHistoryRefs(
	refs []storedRef,
	namespace, name string,
	through int64,
) ([]storedRef, int64, error) {
	latest, err := oneRef(refs, namespace, name)
	if err != nil {
		return nil, 0, err
	}
	for at := 1; at < len(refs); at++ {
		if refs[at].revision > 0 && refs[at-1].revision == refs[at].revision {
			return nil, 0, fmt.Errorf(
				"%w: revision %d",
				errAmbiguousRevision,
				refs[at].revision,
			)
		}
	}
	if through < 1 {
		through = latest.revision
	}
	eligible := make([]storedRef, 0, min(len(refs), historyPageSize+1))
	for _, ref := range refs {
		if ref.revision > through || ref.revision < 1 {
			continue
		}
		eligible = append(eligible, ref)
		if len(eligible) == historyPageSize+1 {
			break
		}
	}
	if len(eligible) <= historyPageSize {
		return eligible, 0, nil
	}
	next := eligible[historyPageSize].revision
	return eligible[:historyPageSize], next, nil
}

func (s *Service) History(
	ctx context.Context,
	namespace, name string,
	through int64,
) (api.HelmHistoryPage, error) {
	if !nameFormat.MatchString(namespace) || !nameFormat.MatchString(name) {
		return api.HelmHistoryPage{}, errors.New("namespace and name must be valid kubernetes names")
	}
	bounded, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()

	refs, err := s.storedRefs(bounded, namespace, releaseSelector(name), maxHistoryRefs)
	if err != nil {
		return api.HelmHistoryPage{}, err
	}
	eligible, next, selectionErr := selectHistoryRefs(refs, namespace, name, through)
	if selectionErr != nil {
		return api.HelmHistoryPage{}, selectionErr
	}
	releases, readable := s.readWithStatus(bounded, eligible)
	revisions := make([]api.HelmRevision, 0, len(releases))
	for at, release := range releases {
		revision := api.HelmRevision{
			Revision:     release.Revision,
			Status:       release.Status,
			ChartVersion: release.ChartVersion,
			AppVersion:   release.AppVersion,
			Updated:      release.Updated,
			Description:  release.Description,
		}
		if !readable[at] {
			revision.Description = "this revision's payload could not be read"
		}
		revisions = append(revisions, revision)
	}
	return api.HelmHistoryPage{Revisions: revisions, Next: next}, nil
}
