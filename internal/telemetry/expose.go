package telemetry

import (
	"bytes"
	"io"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
)

const contentType = "text/plain; version=0.0.4; charset=utf-8"

var (
	labelText = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	helpText  = strings.NewReplacer(`\`, `\\`, "\n", `\n`)
)

type reading struct {
	values  []string
	value   float64
	sum     float64
	count   uint64
	buckets []uint64
}

func ContentType() string {
	return contentType
}

func (r *Registry) Write(dst io.Writer) error {
	body := &bytes.Buffer{}
	for _, one := range r.declared() {
		one.render(body)
	}
	_, err := dst.Write(body.Bytes())
	return err
}

func (r *Registry) declared() []*vector {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := slices.Collect(maps.Values(r.vectors))
	slices.SortFunc(out, func(left, right *vector) int {
		return strings.Compare(left.name, right.name)
	})
	return out
}

func (v *vector) render(into *bytes.Buffer) {
	rows := v.snapshot()
	if len(rows) == 0 {
		return
	}
	into.WriteString("# HELP " + v.name + " " + helpText.Replace(v.help) + "\n")
	into.WriteString("# TYPE " + v.name + " " + v.kind + "\n")
	for _, row := range rows {
		if v.kind == histogramKind {
			v.renderHistogram(into, row)
			continue
		}
		into.WriteString(v.name + labelsOf(pairsOf(v.labels, row.values)) + " " + number(row.value) + "\n")
	}
}

func (v *vector) renderHistogram(into *bytes.Buffer, row reading) {
	pairs := pairsOf(v.labels, row.values)
	for at, bound := range v.buckets {
		bucket := labelsOf(append(slices.Clone(pairs), `le="`+number(bound)+`"`))
		into.WriteString(v.name + "_bucket" + bucket + " " + strconv.FormatUint(row.buckets[at], 10) + "\n")
	}
	unbounded := labelsOf(append(slices.Clone(pairs), `le="+Inf"`))
	into.WriteString(v.name + "_bucket" + unbounded + " " + strconv.FormatUint(row.count, 10) + "\n")
	into.WriteString(v.name + "_sum" + labelsOf(pairs) + " " + number(row.sum) + "\n")
	into.WriteString(v.name + "_count" + labelsOf(pairs) + " " + strconv.FormatUint(row.count, 10) + "\n")
}

func (v *vector) snapshot() []reading {
	v.mu.Lock()
	rows := slices.Collect(maps.Values(v.series))
	v.mu.Unlock()
	out := make([]reading, 0, len(rows))
	for _, one := range rows {
		out = append(out, one.read())
	}
	slices.SortFunc(out, func(left, right reading) int {
		return slices.Compare(left.values, right.values)
	})
	return out
}

func (s *series) read() reading {
	out := reading{values: s.values, buckets: make([]uint64, len(s.buckets))}
	running := uint64(0)
	for at := range s.buckets {
		running += s.buckets[at].Load()
		out.buckets[at] = running
	}
	out.count = s.count.Load()
	out.sum = math.Float64frombits(s.sum.Load())
	out.value = math.Float64frombits(s.value.Load())
	return out
}

func pairsOf(names, values []string) []string {
	out := make([]string, 0, len(names)+1)
	for at, name := range names {
		out = append(out, name+`="`+labelText.Replace(values[at])+`"`)
	}
	return out
}

func labelsOf(pairs []string) string {
	if len(pairs) == 0 {
		return ""
	}
	return "{" + strings.Join(pairs, ",") + "}"
}

func number(value float64) string {
	if math.IsNaN(value) {
		return "NaN"
	}
	if math.IsInf(value, 1) {
		return "+Inf"
	}
	if math.IsInf(value, -1) {
		return "-Inf"
	}
	return strconv.FormatFloat(value, 'g', -1, 64)
}
