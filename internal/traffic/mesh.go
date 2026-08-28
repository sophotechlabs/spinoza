package traffic

import "strings"

type endpoint struct {
	namespace string
	workload  string
}

type mesh struct {
	name    string
	present string
	labeled string
	flows   string
	from    endpoint
	to      endpoint
	verdict string
	hint    string
}

const (
	forwarded = "FORWARDED"
	dropped   = "DROPPED"
)

var cilium = mesh{
	name:    "Cilium Hubble",
	present: `count(hubble_flows_processed_total)`,
	labeled: `count(hubble_flows_processed_total{source_workload!="",destination_workload!=""})`,
	flows: `sum by (source_namespace, source_workload, destination_namespace, destination_workload, verdict) ` +
		`(rate(hubble_flows_processed_total[5m]))`,
	from:    endpoint{namespace: "source_namespace", workload: "source_workload"},
	to:      endpoint{namespace: "destination_namespace", workload: "destination_workload"},
	verdict: "verdict",
	hint: "Cilium is exporting Hubble flow metrics, but they carry no workload labels. " +
		"Add flow:labelsContext=source_namespace,source_workload,destination_namespace,destination_workload " +
		"to hubble.metrics.enabled in the Cilium values, then wait one scrape.",
}

var meshes = []mesh{cilium}

func meshNames() string {
	names := make([]string, 0, len(meshes))
	for _, entry := range meshes {
		names = append(names, entry.name)
	}
	return strings.Join(names, ", ")
}
