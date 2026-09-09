package waste

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const (
	basisPoints = 10000
	namesShown  = 3

	noPods       = "no pods are running here"
	onlyFinished = "every pod here has finished, and a finished pod reserves nothing"
	noSource     = "no usage source is wired up"
	notMeasured  = "nothing measured usage, so this shows what is reserved and not what is used"
)

type Ref struct {
	Namespace string
	Name      string
}

type Reading struct {
	Pods   map[string]api.ResourceUsage
	Source string
	Window string
}

type Meter interface {
	Usage(ctx context.Context, pods []Ref) (Reading, error)
}

type Lister interface {
	ListKind(ctx context.Context, ref api.ObjectRef) ([]*unstructured.Unstructured, error)
}

type Request struct {
	Namespace string
	Meters    []Meter
}

type held struct {
	ref      Ref
	kind     string
	owner    string
	cpu      int64
	mem      int64
	asks     bool
	used     api.ResourceUsage
	measured bool
}

func Build(ctx context.Context, lister Lister, req Request) (api.WasteReport, error) {
	found, err := lister.ListKind(ctx, podsIn(req.Namespace))
	if err != nil {
		return api.WasteReport{}, err
	}
	if len(found) == 0 {
		return api.WasteReport{Namespaces: []api.WasteRow{}, Workloads: []api.WasteRow{}, Reason: noPods}, nil
	}
	owners, notes := ownerIndex(ctx, lister, req.Namespace)
	items := reservations(found, owners)
	if len(items) == 0 {
		return api.WasteReport{Namespaces: []api.WasteRow{}, Workloads: []api.WasteRow{}, Reason: onlyFinished}, nil
	}
	reading, measured, why := measure(ctx, req.Meters, refsOf(items))
	attach(items, reading.Pods)
	out := api.WasteReport{
		Namespaces: ranked(rollup(items, namespaceRow)),
		Workloads:  ranked(rollup(items, workloadRow)),
		Measured:   measured,
	}
	if measured {
		out.Window = reading.Window
		out.Source = reading.Source
		out.PartialOn = partialOn(items, out.Namespaces, out.Workloads)
	}
	out.Reason = reasonOf(measured, why, items, notes)
	return out, nil
}

func podsIn(namespace string) api.ObjectRef {
	return api.ObjectRef{Version: "v1", Resource: "pods", Namespace: namespace}
}

func reservations(found []*unstructured.Unstructured, owners map[string]*unstructured.Unstructured) []held {
	out := make([]held, 0, len(found))
	for _, obj := range found {
		if finished(obj) {
			continue
		}
		cpu, mem, asks := requestsOf(obj)
		kind, name := topOwner(obj, owners)
		out = append(out, held{
			ref:   Ref{Namespace: obj.GetNamespace(), Name: obj.GetName()},
			kind:  kind,
			owner: name,
			cpu:   cpu,
			mem:   mem,
			asks:  asks,
		})
	}
	return out
}

func finished(obj *unstructured.Unstructured) bool {
	phase, _, err := unstructured.NestedString(obj.Object, "status", "phase")
	if err != nil {
		return false
	}
	return phase == "Succeeded" || phase == "Failed"
}

func refsOf(items []held) []Ref {
	out := make([]Ref, 0, len(items))
	for _, item := range items {
		out = append(out, item.ref)
	}
	return out
}

func measure(ctx context.Context, meters []Meter, pods []Ref) (Reading, bool, string) {
	troubles := make([]string, 0, len(meters))
	for _, one := range meters {
		read, err := one.Usage(ctx, pods)
		if err == nil {
			return read, true, ""
		}
		troubles = append(troubles, err.Error())
	}
	if len(troubles) == 0 {
		return Reading{}, false, noSource
	}
	return Reading{}, false, strings.Join(troubles, "; ")
}

func attach(items []held, usage map[string]api.ResourceUsage) {
	for at := range items {
		use, ok := usage[items[at].ref.Namespace+"/"+items[at].ref.Name]
		if !ok {
			continue
		}
		items[at].used = use
		items[at].measured = true
	}
}

func namespaceRow(item held) api.WasteRow {
	return api.WasteRow{Namespace: item.ref.Namespace}
}

func workloadRow(item held) api.WasteRow {
	return api.WasteRow{Kind: item.kind, Namespace: item.ref.Namespace, Name: item.owner}
}

func rollup(items []held, keyOf func(held) api.WasteRow) []api.WasteRow {
	place := map[string]int{}
	rows := []api.WasteRow{}
	whole := []bool{}
	for _, item := range items {
		base := keyOf(item)
		id := base.Kind + "\x00" + base.Namespace + "\x00" + base.Name
		where, seen := place[id]
		if !seen {
			where = len(rows)
			place[id] = where
			rows = append(rows, base)
			whole = append(whole, true)
		}
		into := &rows[where]
		into.Pods++
		into.CPURequested += item.cpu
		into.MemRequested += item.mem
		if !item.measured {
			whole[where] = false
			continue
		}
		into.CPUUsed += item.used.CPUMilli
		into.MemUsed += item.used.MemoryMi
		into.CPUReclaimable += floorAtZero(item.cpu - item.used.CPUMilli)
		into.MemReclaimable += floorAtZero(item.mem - item.used.MemoryMi)
	}
	for at := range rows {
		rows[at].Measured = whole[at]
	}
	return rows
}

func floorAtZero(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func ranked(rows []api.WasteRow) []api.WasteRow {
	var cpuTotal, memTotal int64
	for _, row := range rows {
		cpuTotal += row.CPUReclaimable
		memTotal += row.MemReclaimable
	}
	slices.SortStableFunc(rows, func(left, right api.WasteRow) int {
		byShare := cmp.Compare(score(right, cpuTotal, memTotal), score(left, cpuTotal, memTotal))
		if byShare != 0 {
			return byShare
		}
		byNamespace := strings.Compare(left.Namespace, right.Namespace)
		if byNamespace != 0 {
			return byNamespace
		}
		return strings.Compare(left.Name, right.Name)
	})
	return rows
}

func score(row api.WasteRow, cpuTotal, memTotal int64) int64 {
	return share(row.CPUReclaimable, cpuTotal) + share(row.MemReclaimable, memTotal)
}

func share(part, whole int64) int64 {
	if whole <= 0 {
		return 0
	}
	return part * basisPoints / whole
}

func partialOn(items []held, namespaces, workloads []api.WasteRow) []string {
	seen := map[string]int{}
	for _, item := range items {
		if !item.measured {
			continue
		}
		seen[item.ref.Namespace]++
	}
	names := map[string]bool{}
	for _, row := range namespaces {
		if row.Measured || seen[row.Namespace] > 0 {
			continue
		}
		names[row.Namespace] = true
	}
	for _, row := range workloads {
		if row.Measured || seen[row.Namespace] == 0 {
			continue
		}
		names[row.Namespace+"/"+row.Name] = true
	}
	if len(names) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(names))
}

func reasonOf(measured bool, why string, items []held, notes []string) string {
	parts := make([]string, 0, len(notes)+2)
	if !measured {
		parts = append(parts, notMeasured+": "+why)
	}
	bare := bareNames(items)
	if len(bare) > 0 {
		parts = append(parts, strconv.Itoa(len(bare))+" "+podWord(len(bare))+
			" nothing and cannot be ranked: "+nameList(bare))
	}
	parts = append(parts, notes...)
	return strings.Join(parts, "; ")
}

func bareNames(items []held) []string {
	out := []string{}
	for _, item := range items {
		if item.asks {
			continue
		}
		out = append(out, item.ref.Namespace+"/"+item.ref.Name)
	}
	return out
}

func podWord(count int) string {
	if count == 1 {
		return "pod reserves"
	}
	return "pods reserve"
}

func nameList(names []string) string {
	slices.Sort(names)
	if len(names) <= namesShown {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:namesShown], ", ") +
		", and " + strconv.Itoa(len(names)-namesShown) + " more"
}
