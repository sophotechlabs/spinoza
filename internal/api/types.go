package api

import "errors"

var ErrInternal = errors.New("spinoza could not do that")

type Health struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Context string `json:"context"`
}

type Build struct {
	Version string `json:"version"`
}

type ResourceDescriptor struct {
	Group      string `json:"group"`
	Version    string `json:"version"`
	Resource   string `json:"resource"`
	Kind       string `json:"kind"`
	Namespaced bool   `json:"namespaced"`
	Category   string `json:"category"`
}

type Category struct {
	Name      string               `json:"name"`
	Resources []ResourceDescriptor `json:"resources"`
}

type ContextRef struct {
	Kubeconfig string `json:"kubeconfig"`
	Name       string `json:"name"`
}

type KubeContext struct {
	Cluster   string `json:"cluster"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

type Kubeconfig struct {
	Contexts  []KubeContext `json:"contexts"`
	Error     string        `json:"error,omitempty"`
	Label     string        `json:"label"`
	Path      string        `json:"path"`
	Removable bool          `json:"removable"`
}

const notKnown = "unknown"

const (
	ProtectionProtected = "protected"
	ProtectionOpen      = "open"
	ProtectionUnknown   = notKnown
)

type ContextList struct {
	Current     ContextRef   `json:"current"`
	Error       string       `json:"error,omitempty"`
	Kubeconfigs []Kubeconfig `json:"kubeconfigs"`
	Protection  string       `json:"protection"`
}

type FilePicker struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

type Namespaces struct {
	Names []string `json:"names"`
	Error string   `json:"error,omitempty"`
}

type SearchHit struct {
	Group     string `json:"group"`
	Version   string `json:"version"`
	Resource  string `json:"resource"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

type SearchResults struct {
	Hits      []SearchHit       `json:"hits"`
	Truncated bool              `json:"truncated"`
	Errors    map[string]string `json:"errors,omitempty"`
}

type ViewState struct {
	Window bool `json:"window"`
	Hidden bool `json:"hidden"`
}

type ViewSwitch struct {
	Switched bool   `json:"switched"`
	Reason   string `json:"reason,omitempty"`
}

type Settings struct {
	Values map[string]string `json:"values"`
}

type LocalShell struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

type TerminalSize struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

type PickedFile struct {
	Path string `json:"path"`
}

type ResourceCounts struct {
	Counts  map[string]int    `json:"counts"`
	Failing map[string]int    `json:"failing,omitempty"`
	Errors  map[string]string `json:"errors,omitempty"`
}

type ResourceCatalog struct {
	Categories []Category `json:"categories"`
	Error      string     `json:"error,omitempty"`
}

type NodeSummary struct {
	Total               int   `json:"total"`
	Ready               int   `json:"ready"`
	Unschedulable       int   `json:"unschedulable"`
	CPUAllocatableMilli int64 `json:"cpuAllocatableMilli"`
	CPUUsedMilli        int64 `json:"cpuUsedMilli"`
	MemAllocatableMi    int64 `json:"memAllocatableMi"`
	MemUsedMi           int64 `json:"memUsedMi"`
	UsageKnown          bool  `json:"usageKnown"`
}

type PodSummary struct {
	Total     int  `json:"total"`
	Running   int  `json:"running"`
	Pending   int  `json:"pending"`
	Failed    int  `json:"failed"`
	Succeeded int  `json:"succeeded"`
	Known     bool `json:"known"`
}

type OverviewEvent struct {
	Namespace string `json:"namespace"`
	Object    string `json:"object"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	Count     int64  `json:"count"`
	LastSeen  string `json:"lastSeen"`
}

type ClusterOverview struct {
	Version  string          `json:"version"`
	Nodes    NodeSummary     `json:"nodes"`
	Pods     PodSummary      `json:"pods"`
	Warnings []OverviewEvent `json:"warnings"`
	Error    string          `json:"error,omitempty"`
}

type HelmRelease struct {
	Name         string     `json:"name"`
	Namespace    string     `json:"namespace"`
	Chart        string     `json:"chart"`
	ChartVersion string     `json:"chartVersion"`
	AppVersion   string     `json:"appVersion"`
	Latest       string     `json:"latest,omitempty"`
	Outdated     bool       `json:"outdated,omitempty"`
	Revision     int64      `json:"revision"`
	Status       string     `json:"status"`
	Updated      string     `json:"updated"`
	Description  string     `json:"description,omitempty"`
	FluxRef      *ObjectRef `json:"fluxRef,omitempty"`
}

type HelmReleases struct {
	Releases []HelmRelease `json:"releases"`
	Error    string        `json:"error,omitempty"`
}

type HelmRevision struct {
	Revision     int64  `json:"revision"`
	Status       string `json:"status"`
	ChartVersion string `json:"chartVersion"`
	AppVersion   string `json:"appVersion"`
	Updated      string `json:"updated"`
	Description  string `json:"description,omitempty"`
}

type HelmResource struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace,omitempty"`
	Group      string `json:"group,omitempty"`
	Version    string `json:"version,omitempty"`
	Resource   string `json:"resource,omitempty"`
}

type HelmReleaseDetail struct {
	Release       HelmRelease    `json:"release"`
	Driver        string         `json:"driver"`
	FirstDeployed string         `json:"firstDeployed,omitempty"`
	Values        string         `json:"values"`
	Notes         string         `json:"notes"`
	Manifest      string         `json:"manifest"`
	Resources     []HelmResource `json:"resources"`
	History       []HelmRevision `json:"history"`
	Error         string         `json:"error,omitempty"`
}

type HelmSupport struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
	Binary    string `json:"binary"`
}

type HelmActionResult struct {
	Action   string `json:"action"`
	Message  string `json:"message"`
	Revision int64  `json:"revision,omitempty"`
	DryRun   bool   `json:"dryRun,omitempty"`
	Manifest string `json:"manifest,omitempty"`
}

type HelmRepoVersions struct {
	Name     string   `json:"name,omitempty"`
	URL      string   `json:"url"`
	OCI      bool     `json:"oci,omitempty"`
	Versions []string `json:"versions"`
}

type HelmChartVersions struct {
	Chart string             `json:"chart"`
	Repos []HelmRepoVersions `json:"repos"`
	Error string             `json:"error,omitempty"`
}

type Column struct {
	Name   string `json:"name"`
	Render string `json:"render,omitempty"`
}

type ContainerState struct {
	Name      string `json:"name"`
	State     string `json:"state"`
	Reason    string `json:"reason,omitempty"`
	Ready     bool   `json:"ready"`
	Restarts  int64  `json:"restarts"`
	Init      bool   `json:"init"`
	Ephemeral bool   `json:"ephemeral,omitempty"`
}

type Row struct {
	UID        string           `json:"uid"`
	Name       string           `json:"name"`
	Namespace  string           `json:"namespace"`
	CreatedAt  string           `json:"createdAt"`
	Cells      []string         `json:"cells"`
	Containers []ContainerState `json:"containers,omitempty"`
}

type ObjectRef struct {
	Group     string `json:"group"`
	Version   string `json:"version"`
	Resource  string `json:"resource"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

type OwnerRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	UID  string `json:"uid"`
}

type Condition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
	Updated string `json:"updated,omitempty"`
}

type ObjectDetail struct {
	APIVersion  string            `json:"apiVersion"`
	Kind        string            `json:"kind"`
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	UID         string            `json:"uid"`
	CreatedAt   string            `json:"createdAt"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Owners      []OwnerRef        `json:"owners,omitempty"`
	Conditions  []Condition       `json:"conditions,omitempty"`
	Containers  []string          `json:"containers,omitempty"`
	Suspended   *bool             `json:"suspended,omitempty"`
	Replicas    *int64            `json:"replicas,omitempty"`
	Schedulable *bool             `json:"schedulable,omitempty"`
	HandledAt   string            `json:"handledAt,omitempty"`
	Ports       []ObjectPort      `json:"ports,omitempty"`
	Event       *ObjectEvent      `json:"event,omitempty"`
	YAML        string            `json:"yaml"`
}

type ObjectEvent struct {
	Type      string `json:"type,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Message   string `json:"message,omitempty"`
	Object    string `json:"object,omitempty"`
	Source    string `json:"source,omitempty"`
	Count     int64  `json:"count,omitempty"`
	FirstSeen string `json:"firstSeen,omitempty"`
	LastSeen  string `json:"lastSeen,omitempty"`
}

type ObjectPort struct {
	Name     string `json:"name,omitempty"`
	Port     int32  `json:"port"`
	Protocol string `json:"protocol,omitempty"`
}

type PortForward struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	Pod        string `json:"pod,omitempty"`
	RemotePort int32  `json:"remotePort"`
	LocalPort  int32  `json:"localPort"`
	State      string `json:"state"`
	Error      string `json:"error,omitempty"`
	StartedAt  string `json:"startedAt"`
}

const (
	ExecChannelStdin  = 0x00
	ExecChannelStdout = 0x01
	ExecChannelStderr = 0x02
	ExecChannelError  = 0x03
	ExecChannelResize = 0x04
)

const (
	ShellUnknown = notKnown
	ShellPresent = "present"
	ShellAbsent  = "absent"
)

type FluxActionResult struct {
	Action      string `json:"action"`
	RequestedAt string `json:"requestedAt,omitempty"`
}

const (
	OutcomeEvict   = "evict"
	OutcomeEvicted = "evicted"
	OutcomeBlocked = "blocked"
	OutcomeSkipped = "skipped"
	OutcomeFailed  = "failed"
)

type PodOutcome struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Outcome   string `json:"outcome"`
	Reason    string `json:"reason,omitempty"`
}

type ActionResult struct {
	Action  string       `json:"action"`
	Message string       `json:"message"`
	DryRun  bool         `json:"dryRun,omitempty"`
	Pods    []PodOutcome `json:"pods,omitempty"`
}

type DebugSupport struct {
	Namespace string `json:"namespace"`
	Pod       string `json:"pod,omitempty"`
	Allowed   bool   `json:"allowed"`
	Reason    string `json:"reason,omitempty"`
	Image     string `json:"image"`
}

type DebugSession struct {
	Container string `json:"container"`
	Created   bool   `json:"created"`
	Image     string `json:"image"`
	Profile   string `json:"profile"`
	Target    string `json:"target,omitempty"`
}

type ExecSupport struct {
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	Container string `json:"container"`
	Image     string `json:"image,omitempty"`
	Shell     string `json:"shell"`
}

type Event struct {
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	Source    string `json:"source"`
	Count     int64  `json:"count"`
	FirstSeen string `json:"firstSeen"`
	LastSeen  string `json:"lastSeen"`
}

type ClientMsg struct {
	Type      string `json:"type"`
	SubID     string `json:"subId"`
	Group     string `json:"group"`
	Version   string `json:"version"`
	Resource  string `json:"resource"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Container string `json:"container"`
	TailLines int64  `json:"tailLines"`
	Follow    bool   `json:"follow"`
	Limit     int    `json:"limit"`
}

type ServerMsg struct {
	Type       string      `json:"type"`
	SubID      string      `json:"subId,omitempty"`
	Columns    []Column    `json:"columns,omitempty"`
	Namespaced bool        `json:"namespaced,omitempty"`
	Rows       []Row       `json:"rows,omitempty"`
	Row        *Row        `json:"row,omitempty"`
	UID        string      `json:"uid,omitempty"`
	Total      int         `json:"total,omitempty"`
	Limit      int         `json:"limit,omitempty"`
	Lines      []string    `json:"lines,omitempty"`
	Message    string      `json:"message,omitempty"`
	Changes    []RowChange `json:"changes,omitempty"`
}

type Snapshot struct {
	Type       string   `json:"type"`
	SubID      string   `json:"subId"`
	Columns    []Column `json:"columns"`
	Namespaced bool     `json:"namespaced"`
	Rows       []Row    `json:"rows"`
	Total      int      `json:"total"`
	Limit      int      `json:"limit"`
}

type RowChanged struct {
	Type  string `json:"type"`
	SubID string `json:"subId"`
	Row   Row    `json:"row"`
}

type RowDeleted struct {
	Type  string `json:"type"`
	SubID string `json:"subId"`
	UID   string `json:"uid"`
}

type RowChange struct {
	Type string `json:"type"`
	Row  Row    `json:"row,omitzero"`
	UID  string `json:"uid,omitempty"`
}

type RowBatch struct {
	Type    string      `json:"type"`
	SubID   string      `json:"subId"`
	Changes []RowChange `json:"changes"`
}

type LogLines struct {
	Type  string   `json:"type"`
	SubID string   `json:"subId"`
	Lines []string `json:"lines"`
}

type LogEnd struct {
	Type  string `json:"type"`
	SubID string `json:"subId"`
}

type FeedError struct {
	Type    string `json:"type"`
	SubID   string `json:"subId"`
	Message string `json:"message"`
}

type GraphNode struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Group     string `json:"group"`
	Version   string `json:"version"`
	Resource  string `json:"resource"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Ready     string `json:"ready"`
	Category  string `json:"category"`
}

type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

type Graph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
	Error string      `json:"error,omitempty"`
}

type ArgoApp struct {
	Kind        string `json:"kind"`
	Group       string `json:"group"`
	Version     string `json:"version"`
	Resource    string `json:"resource"`
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Project     string `json:"project"`
	Sync        string `json:"sync"`
	Health      string `json:"health"`
	Revision    string `json:"revision"`
	Repo        string `json:"repo"`
	Path        string `json:"path"`
	Destination string `json:"destination"`
	Message     string `json:"message"`
	Owner       string `json:"owner,omitempty"`
	CreatedAt   string `json:"createdAt"`
}

type ArgoDashboard struct {
	Apps            []ArgoApp `json:"apps"`
	ApplicationSets []ArgoApp `json:"applicationSets"`
	Projects        []ArgoApp `json:"projects"`
	Error           string    `json:"error,omitempty"`
}

type FluxResource struct {
	Kind      string `json:"kind"`
	Group     string `json:"group"`
	Version   string `json:"version"`
	Resource  string `json:"resource"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Ready     string `json:"ready"`
	Suspended bool   `json:"suspended"`
	Revision  string `json:"revision"`
	Latest    string `json:"latest,omitempty"`
	Outdated  bool   `json:"outdated,omitempty"`
	Source    string `json:"source"`
	Message   string `json:"message"`
	CreatedAt string `json:"createdAt"`
}

type FluxGroup struct {
	Name      string         `json:"name"`
	Ready     int            `json:"ready"`
	Reporting int            `json:"reporting"`
	Total     int            `json:"total"`
	Resources []FluxResource `json:"resources"`
}

type FluxController struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Ready     bool   `json:"ready"`
	Replicas  int    `json:"replicas"`
	Wanted    int    `json:"wanted"`
	Namespace string `json:"namespace"`
}

type FluxSync struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Source    string `json:"source"`
	URL       string `json:"url"`
	Ref       string `json:"ref"`
	Path      string `json:"path"`
	Revision  string `json:"revision"`
	Ready     bool   `json:"ready"`
}

type FluxUsage struct {
	CPUMilli        int64 `json:"cpuMilli"`
	MemoryMi        int64 `json:"memoryMi"`
	CPURequestMilli int64 `json:"cpuRequestMilli"`
	MemRequestMi    int64 `json:"memRequestMi"`
	CPULimitMilli   int64 `json:"cpuLimitMilli"`
	MemLimitMi      int64 `json:"memLimitMi"`
	Known           bool  `json:"known"`
}

type FluxOverview struct {
	Ready        bool             `json:"ready"`
	Summary      string           `json:"summary"`
	Namespace    string           `json:"namespace"`
	Kubernetes   string           `json:"kubernetes"`
	Nodes        int              `json:"nodes"`
	Operator     string           `json:"operator,omitempty"`
	Distribution string           `json:"distribution,omitempty"`
	Controllers  []FluxController `json:"controllers"`
	Sync         FluxSync         `json:"sync"`
	Usage        FluxUsage        `json:"usage"`
	Error        string           `json:"error,omitempty"`
}

type FluxDashboard struct {
	Groups []FluxGroup `json:"groups"`
	Error  string      `json:"error,omitempty"`
}

type ResourceUsage struct {
	CPUMilli   int64 `json:"cpuMilli"`
	MemoryMi   int64 `json:"memoryMi"`
	CPUPercent int64 `json:"cpuPercent"`
	MemPercent int64 `json:"memPercent"`
}

type MetricPoint struct {
	At    int64   `json:"at"`
	Value float64 `json:"value"`
}

type MetricHistory struct {
	Namespace string        `json:"namespace"`
	Pod       string        `json:"pod"`
	Source    string        `json:"source,omitempty"`
	CPU       []MetricPoint `json:"cpu"`
	Memory    []MetricPoint `json:"memory"`
}

type Metrics struct {
	Pods  map[string]ResourceUsage `json:"pods"`
	Nodes map[string]ResourceUsage `json:"nodes"`
	Error string                   `json:"error,omitempty"`
}
