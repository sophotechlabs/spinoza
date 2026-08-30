package resources

import (
	"context"
	"hash/fnv"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	Added   = "added"
	Changed = "changed"
	Removed = "removed"
)

type Note struct {
	At        time.Time
	Verb      string
	Group     string
	Version   string
	Resource  string
	Kind      string
	Namespace string
	Name      string
	UID       string
	Cells     []string
}

// Timeline takes what changed. It is called from the informer's own callback,
// so it must return without waiting for a disk.
type Timeline interface {
	Note(note Note)
}

type Kind struct {
	Group    string
	Resource string
}

// Record watches a fixed set of kinds rather than whatever happens to be on
// screen, so what the timeline holds does not depend on what was clicked.
func (m *Manager) Record(ctx context.Context, into Timeline, kinds []Kind) {
	descs := m.recorded(kinds)
	wanted := map[schema.GroupVersionResource]struct{}{}
	for _, desc := range descs {
		wanted[gvrOf(desc)] = struct{}{}
	}
	m.noteMu.Lock()
	m.notes = into
	m.noted = wanted
	m.noteMu.Unlock()
	m.Warm(ctx, descs)
}

// A kind the cluster does not have is skipped rather than refused: the same
// timeline setting has to mean something on every cluster it is turned on for.
func (m *Manager) recorded(kinds []Kind) []api.ResourceDescriptor {
	known := m.descriptors()
	out := make([]api.ResourceDescriptor, 0, len(kinds))
	for _, want := range kinds {
		for _, desc := range known {
			if desc.Group == want.Group && desc.Resource == want.Resource {
				out = append(out, desc)
				break
			}
		}
	}
	slices.SortFunc(out, func(left, right api.ResourceDescriptor) int {
		return strings.Compare(gvrOf(left).String(), gvrOf(right).String())
	})
	return out
}

func (m *Manager) StopRecording() {
	m.noteMu.Lock()
	was := m.noted
	m.notes = nil
	m.noted = nil
	m.noteMu.Unlock()
	for gvr := range was {
		m.unpin(streamKey{gvr: gvr})
	}
}

func (m *Manager) recording(gvr schema.GroupVersionResource) Timeline {
	m.noteMu.Lock()
	defer m.noteMu.Unlock()
	if m.notes == nil {
		return nil
	}
	_, wanted := m.noted[gvr]
	if !wanted {
		return nil
	}
	return m.notes
}

func gvrOf(desc api.ResourceDescriptor) schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    desc.Group,
		Version:  desc.Version,
		Resource: desc.Resource,
	}
}

// note writes when the row changes, not when the object does: a reconcile that
// only rewrites a condition message shows nothing on screen and is not history.
func (st *stream) note(verb string, obj *unstructured.Unstructured, row api.Row) {
	into := st.owner.recording(st.gvr)
	if into == nil {
		return
	}
	fresh := st.rowIsNew(verb, row)
	if !fresh {
		return
	}
	// What was already on the cluster when recording started is not something
	// that happened; the first listing seeds the shapes and writes nothing.
	// The handler's own registration is what says the listing is behind us —
	// the informer's store syncs before its listeners are told anything.
	if !st.delivered() {
		return
	}
	into.Note(Note{
		At:        time.Now().UTC(),
		Verb:      verb,
		Group:     st.gvr.Group,
		Version:   st.gvr.Version,
		Resource:  st.gvr.Resource,
		Kind:      st.kind,
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
		UID:       string(obj.GetUID()),
		Cells:     row.Cells,
	})
}

func (st *stream) delivered() bool {
	handler := st.handler.Load()
	if handler == nil {
		return false
	}
	return (*handler).HasSynced()
}

func (st *stream) rowIsNew(verb string, row api.Row) bool {
	st.seenMu.Lock()
	defer st.seenMu.Unlock()
	if verb == Removed {
		delete(st.seen, row.UID)
		return true
	}
	shape := shapeOf(row)
	if st.seen == nil {
		st.seen = map[string]uint64{}
	}
	was, known := st.seen[row.UID]
	st.seen[row.UID] = shape
	if !known {
		return true
	}
	return was != shape
}

func shapeOf(row api.Row) uint64 {
	digest := fnv.New64a()
	for _, cell := range row.Cells {
		_, _ = digest.Write([]byte(cell))
		_, _ = digest.Write([]byte{0})
	}
	for _, one := range row.Containers {
		_, _ = digest.Write([]byte(one.Name + "\x00" + one.State + "\x00" + one.Reason))
		_, _ = digest.Write([]byte(strconv.FormatBool(one.Ready)))
		_, _ = digest.Write([]byte(strconv.FormatInt(one.Restarts, 10)))
		_, _ = digest.Write([]byte{0})
	}
	return digest.Sum64()
}
