package api

import "errors"

var ErrNotOpen = errors.New("that cluster is not open")

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

type CustomColumn struct {
	Name string `json:"name"`
	Path string `json:"path"`
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

type OpenCluster struct {
	ID         string `json:"id"`
	Context    string `json:"context"`
	Kubeconfig string `json:"kubeconfig,omitempty"`
	Active     bool   `json:"active"`
	Color      int    `json:"color"`
	Label      string `json:"label,omitempty"`
	Grouping   string `json:"grouping,omitempty"`
	Reopen     bool   `json:"reopen"`
	Timeline   string `json:"timeline,omitempty"`
	Protection string `json:"protection"`
	Reachable  bool   `json:"reachable"`
	Reason     string `json:"reason,omitempty"`
}

const ClusterColors = 8

type RememberedCluster struct {
	ID         string `json:"id"`
	Context    string `json:"context"`
	Kubeconfig string `json:"kubeconfig,omitempty"`
}

type ClusterList struct {
	Clusters   []OpenCluster       `json:"clusters"`
	Remembered []RememberedCluster `json:"remembered"`
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
	Cluster   string `json:"cluster,omitempty"`
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

const (
	HistoryDone    = "done"
	HistoryRefused = "refused"
	HistoryFailed  = OutcomeFailed
)

const HistoryOff = "spinoza is not recording what it does"

const (
	HistoryAll    = "all"
	HistoryAction = "action"
	HistoryChange = "change"
)

type HistoryEntry struct {
	ID        int64  `json:"id"`
	Source    string `json:"source"`
	Cluster   string `json:"cluster,omitempty"`
	At        string `json:"at"`
	Verb      string `json:"verb"`
	Group     string `json:"group,omitempty"`
	Version   string `json:"version,omitempty"`
	Resource  string `json:"resource,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	Detail    string `json:"detail,omitempty"`
	Was       string `json:"was,omitempty"`
	Outcome   string `json:"outcome"`
	Message   string `json:"message,omitempty"`
}

type History struct {
	Entries []HistoryEntry `json:"entries"`
	More    bool           `json:"more,omitempty"`
	Dropped int            `json:"dropped,omitempty"`
	Next    int64          `json:"next,omitempty"`
	Reason  string         `json:"reason,omitempty"`
}

type Memory struct {
	HeapMi int64 `json:"heapMi"`
	SysMi  int64 `json:"sysMi"`
}

// RBACRule is one policy rule as written, so a reader can see the wildcard
// rather than only its consequence.
type RBACRule struct {
	Verbs     []string `json:"verbs,omitempty"`
	Groups    []string `json:"groups,omitempty"`
	Resources []string `json:"resources,omitempty"`
	Names     []string `json:"names,omitempty"`
	URLs      []string `json:"urls,omitempty"`
}

// RBACGrant is one binding's contribution to a subject. An empty namespace
// means the grant applies everywhere.
type RBACGrant struct {
	Binding     string     `json:"binding"`
	BindingKind string     `json:"bindingKind"`
	Role        string     `json:"role"`
	RoleKind    string     `json:"roleKind"`
	Namespace   string     `json:"namespace,omitempty"`
	Rules       []RBACRule `json:"rules,omitempty"`
	Missing     bool       `json:"missing,omitempty"`
	Aggregated  bool       `json:"aggregated,omitempty"`
}

type RBACSubject struct {
	Kind       string      `json:"kind"`
	Name       string      `json:"name"`
	Namespace  string      `json:"namespace,omitempty"`
	Label      string      `json:"label"`
	Powers     []string    `json:"powers,omitempty"`
	Namespaces []string    `json:"namespaces,omitempty"`
	Grants     []RBACGrant `json:"grants"`
}

type RBACIndex struct {
	Subjects []RBACSubject `json:"subjects"`
	Absent   []string      `json:"absent,omitempty"`
	Dropped  int           `json:"dropped,omitempty"`
	Error    string        `json:"error,omitempty"`
}

type Settings struct {
	Values map[string]string `json:"values"`
}

type LocalShell struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

type Capabilities struct {
	Helm       HelmSupport    `json:"helm"`
	Traffic    TrafficSupport `json:"traffic"`
	LocalShell LocalShell     `json:"localShell"`
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
	ByPhase []string          `json:"byPhase,omitempty"`
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
	Total     int      `json:"total"`
	Running   int      `json:"running"`
	Pending   int      `json:"pending"`
	Failed    int      `json:"failed"`
	Succeeded int      `json:"succeeded"`
	Known     bool     `json:"known"`
	Capped    []string `json:"capped"`
}

type OverviewEvent struct {
	Namespace string `json:"namespace"`
	Object    string `json:"object"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	Count     int64  `json:"count"`
	LastSeen  string `json:"lastSeen"`
}

type GitopsController struct {
	Controller string `json:"controller"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Ready      int    `json:"ready"`
	Wanted     int    `json:"wanted"`
}

type ClusterOverview struct {
	Version     string             `json:"version"`
	Nodes       NodeSummary        `json:"nodes"`
	Pods        PodSummary         `json:"pods"`
	Warnings    []OverviewEvent    `json:"warnings"`
	Controllers []GitopsController `json:"controllers,omitempty"`
	Error       string             `json:"error,omitempty"`
}

// FleetCluster is one cluster's line in the fleet overview: what a person
// scanning several clusters needs before deciding which one to open.
type FleetCluster struct {
	Cluster  string      `json:"cluster"`
	Context  string      `json:"context"`
	Version  string      `json:"version"`
	Nodes    NodeSummary `json:"nodes"`
	Pods     PodSummary  `json:"pods"`
	Warnings int         `json:"warnings"`
	Reason   string      `json:"reason,omitempty"`
}

// FleetKind is one resource type across the fleet: how many exist and how many
// are unhealthy, per cluster and in total.
type FleetKind struct {
	Key        string         `json:"key"`
	Total      int            `json:"total"`
	Failing    int            `json:"failing,omitempty"`
	PerCluster map[string]int `json:"perCluster"`
}

type FleetInventory struct {
	Kinds []FleetKind `json:"kinds"`
	Error string      `json:"error,omitempty"`
}

// FleetImage is one container image and everywhere it runs. Images are the
// thing a fleet drifts on that nothing else surfaces.
type FleetImage struct {
	Image    string   `json:"image"`
	Repo     string   `json:"repo"`
	Tag      string   `json:"tag,omitempty"`
	Pods     int      `json:"pods"`
	Clusters []string `json:"clusters"`
	Skew     []string `json:"skew,omitempty"`
}

type FleetImages struct {
	Images []FleetImage `json:"images"`
	Error  string       `json:"error,omitempty"`
}

type FleetOverview struct {
	Clusters []FleetCluster `json:"clusters"`
	Nodes    NodeSummary    `json:"nodes"`
	Pods     PodSummary     `json:"pods"`
	Error    string         `json:"error,omitempty"`
}

const (
	SeverityFatal    = "fatal"
	SeverityDegraded = "degraded"
	SeverityWarning  = "warning"
	SeverityInfo     = "info"
)

type IssueChild struct {
	Object   ObjectRef `json:"object"`
	Kind     string    `json:"kind"`
	Severity string    `json:"severity"`
	Detail   string    `json:"detail"`
	Since    string    `json:"since"`
}

type Issue struct {
	ID        string       `json:"id"`
	Cluster   string       `json:"cluster,omitempty"`
	Severity  string       `json:"severity"`
	Detector  string       `json:"detector"`
	Title     string       `json:"title"`
	Detail    string       `json:"detail"`
	Action    string       `json:"action"`
	Change    string       `json:"change,omitempty"`
	ChangedAt string       `json:"changedAt,omitempty"`
	Uncertain bool         `json:"uncertain,omitempty"`
	Object    ObjectRef    `json:"object"`
	Kind      string       `json:"kind"`
	Since     string       `json:"since"`
	Folded    int          `json:"folded"`
	Children  []IssueChild `json:"children,omitempty"`
}

type IssueQueue struct {
	Rows    []Issue `json:"rows"`
	Dropped int     `json:"dropped"`
	Next    string  `json:"next,omitempty"`
	Error   string  `json:"error,omitempty"`
}

type HelmRelease struct {
	Cluster      string     `json:"cluster,omitempty"`
	Skew         string     `json:"skew,omitempty"`
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

type HelmChartHit struct {
	Chart       string `json:"chart"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	Repo        string `json:"repo,omitempty"`
	URL         string `json:"url"`
}

type HelmChartSearch struct {
	Query     string         `json:"query"`
	Hits      []HelmChartHit `json:"hits"`
	Truncated bool           `json:"truncated,omitempty"`
	Error     string         `json:"error,omitempty"`
}

type HelmChartValues struct {
	Chart   string `json:"chart"`
	Version string `json:"version"`
	Values  string `json:"values"`
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
	Terminating bool              `json:"terminating,omitempty"`
	Finalizers  []string          `json:"finalizers,omitempty"`
	ManagedBy   *GitopsOwner      `json:"managedBy,omitempty"`
	Source      string            `json:"source,omitempty"`
	Consumers   []GitopsOwner     `json:"consumers,omitempty"`
	Replicas    *int64            `json:"replicas,omitempty"`
	Schedulable *bool             `json:"schedulable,omitempty"`
	HandledAt   string            `json:"handledAt,omitempty"`
	Ports       []ObjectPort      `json:"ports,omitempty"`
	Event       *ObjectEvent      `json:"event,omitempty"`
	Data        []DataEntry       `json:"data,omitempty"`
	YAML        string            `json:"yaml"`
}

type Comparison struct {
	Left         string `json:"left"`
	Right        string `json:"right"`
	LeftContext  string `json:"leftContext"`
	RightContext string `json:"rightContext"`
	Identical    bool   `json:"identical"`
	Missing      string `json:"missing,omitempty"`
}

const (
	VerdictSame      = "same"
	VerdictDiffers   = "differs"
	VerdictOnlyHere  = "onlyHere"
	VerdictOnlyThere = "onlyThere"
)

type KindDiff struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	Verdict   string `json:"verdict"`
	Lines     int    `json:"lines,omitempty"`
}

type KindComparison struct {
	Resource      string     `json:"resource"`
	LeftContext   string     `json:"leftContext"`
	RightContext  string     `json:"rightContext"`
	Namespace     string     `json:"namespace,omitempty"`
	Objects       []KindDiff `json:"objects"`
	Same          int        `json:"same"`
	Differs       int        `json:"differs"`
	OnlyHere      int        `json:"onlyHere"`
	OnlyThere     int        `json:"onlyThere"`
	MatchedByName bool       `json:"matchedByName,omitempty"`
}

type NodeShellSupport struct {
	Node      string `json:"node"`
	Enabled   bool   `json:"enabled"`
	Allowed   bool   `json:"allowed"`
	Reason    string `json:"reason,omitempty"`
	Image     string `json:"image"`
	Namespace string `json:"namespace"`
}

type NodeShellSession struct {
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	Container string `json:"container"`
	Node      string `json:"node"`
	Image     string `json:"image"`
}

type DataEntry struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Bytes  int    `json:"bytes"`
	Binary bool   `json:"binary,omitempty"`
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

type ArgoActionResult struct {
	Action string `json:"action"`
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

// Access carries refusals only; absent means permitted or unanswerable.
type Access struct {
	Refused []Refusal `json:"refused"`
}

type Refusal struct {
	Capability string `json:"capability"`
	Reason     string `json:"reason"`
}

type AccessQuery struct {
	Capability string      `json:"capability"`
	Refs       []ObjectRef `json:"refs"`
}

// BulkAccess indexes refusals by their place in the list asked about.
type BulkAccess struct {
	Refused []RowRefusal `json:"refused"`
}

type RowRefusal struct {
	At     int    `json:"at"`
	Reason string `json:"reason"`
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

type Failure struct {
	Message string `json:"message"`
}

type RowFilter struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

type ClientMsg struct {
	Type      string      `json:"type"`
	SubID     string      `json:"subId"`
	Cluster   string      `json:"cluster,omitempty"`
	Group     string      `json:"group"`
	Version   string      `json:"version"`
	Resource  string      `json:"resource"`
	Namespace string      `json:"namespace"`
	Name      string      `json:"name"`
	Container string      `json:"container"`
	TailLines int64       `json:"tailLines"`
	Follow    bool        `json:"follow"`
	Limit     int         `json:"limit"`
	Filters   []RowFilter `json:"filters,omitempty"`
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
	Source     string      `json:"source,omitempty"`
	Attached   int         `json:"attached,omitempty"`
	Matched    int         `json:"matched,omitempty"`
	Message    string      `json:"message,omitempty"`
	Context    string      `json:"context,omitempty"`
	Cluster    string      `json:"cluster,omitempty"`
	Reachable  bool        `json:"reachable,omitempty"`
	Reason     string      `json:"reason,omitempty"`
	Changes    []RowChange `json:"changes,omitempty"`
}

type ClusterHealth struct {
	Type      string `json:"type"`
	Cluster   string `json:"cluster,omitempty"`
	Reachable bool   `json:"reachable"`
	Reason    string `json:"reason,omitempty"`
}

type ContextChanged struct {
	Type    string `json:"type"`
	Cluster string `json:"cluster,omitempty"`
	Context string `json:"context"`
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
	Type   string   `json:"type"`
	SubID  string   `json:"subId"`
	Lines  []string `json:"lines"`
	Source string   `json:"source,omitempty"`
}

type LogOpened struct {
	Type     string `json:"type"`
	SubID    string `json:"subId"`
	Attached int    `json:"attached"`
	Matched  int    `json:"matched"`
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
	Contains  int    `json:"contains"`
	Unhealthy int    `json:"unhealthy"`
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

type TrafficNode struct {
	ID        string `json:"id"`
	Namespace string `json:"namespace"`
	Workload  string `json:"workload"`
}

type TrafficEdge struct {
	From    string  `json:"from"`
	To      string  `json:"to"`
	Rate    float64 `json:"rate"`
	Dropped float64 `json:"dropped"`
}

type TrafficGraph struct {
	Source    string        `json:"source"`
	Nodes     []TrafficNode `json:"nodes"`
	Edges     []TrafficEdge `json:"edges"`
	Folded    bool          `json:"folded,omitempty"`
	Workloads int           `json:"workloads,omitempty"`
	Error     string        `json:"error,omitempty"`
}

type TrafficSupport struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
	Source    string `json:"source,omitempty"`
}

type ArgoApp struct {
	Kind        string `json:"kind"`
	Automation  string `json:"automation,omitempty"`
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

const (
	ControllerArgo = "argocd"
	ControllerFlux = "flux"
)

const (
	SyncModeAuto      = "auto"
	SyncModeManual    = "manual"
	SyncModeSuspended = "suspended"
)

type GitopsOwner struct {
	Controller string    `json:"controller"`
	Kind       string    `json:"kind"`
	Ref        ObjectRef `json:"ref"`
}

type GitopsSource struct {
	Repo        string `json:"repo,omitempty"`
	Path        string `json:"path,omitempty"`
	Target      string `json:"target,omitempty"`
	Destination string `json:"destination,omitempty"`
	Project     string `json:"project,omitempty"`
	SyncMode    string `json:"syncMode"`
	Policy      string `json:"policy,omitempty"`
}

type GitopsState struct {
	Sync      string `json:"sync,omitempty"`
	Health    string `json:"health,omitempty"`
	Revision  string `json:"revision,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
	SyncedAt  string `json:"syncedAt,omitempty"`
	Message   string `json:"message,omitempty"`
}

type GitopsIssue struct {
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Detail   string `json:"detail,omitempty"`
	Subject  string `json:"subject,omitempty"`
}

type FieldDrift struct {
	Path     string `json:"path"`
	Declared string `json:"declared"`
	Live     string `json:"live"`
}

type GitopsResource struct {
	Group       string       `json:"group,omitempty"`
	Version     string       `json:"version,omitempty"`
	Resource    string       `json:"resource,omitempty"`
	Kind        string       `json:"kind"`
	Name        string       `json:"name"`
	Namespace   string       `json:"namespace,omitempty"`
	Sync        string       `json:"sync,omitempty"`
	Health      string       `json:"health,omitempty"`
	Message     string       `json:"message,omitempty"`
	Terminating bool         `json:"terminating,omitempty"`
	Finalizers  []string     `json:"finalizers,omitempty"`
	Drift       []FieldDrift `json:"drift,omitempty"`
	DriftOwners bool         `json:"driftOwners,omitempty"`
	DriftNote   string       `json:"driftNote,omitempty"`
	Events      []Event      `json:"events,omitempty"`
}

type GitopsDeployment struct {
	ID          int64  `json:"id"`
	Revision    string `json:"revision"`
	Source      string `json:"source,omitempty"`
	StartedAt   string `json:"startedAt,omitempty"`
	DeployedAt  string `json:"deployedAt,omitempty"`
	InitiatedBy string `json:"initiatedBy,omitempty"`
	Automated   bool   `json:"automated,omitempty"`
}

type GitopsOperation struct {
	Phase       string `json:"phase"`
	Running     bool   `json:"running,omitempty"`
	Message     string `json:"message,omitempty"`
	Cause       string `json:"cause,omitempty"`
	Revision    string `json:"revision,omitempty"`
	StartedAt   string `json:"startedAt,omitempty"`
	FinishedAt  string `json:"finishedAt,omitempty"`
	InitiatedBy string `json:"initiatedBy,omitempty"`
}

type GitopsApp struct {
	Ref         ObjectRef          `json:"ref"`
	Controller  string             `json:"controller"`
	Terminating bool               `json:"terminating,omitempty"`
	Kind        string             `json:"kind"`
	Name        string             `json:"name"`
	Namespace   string             `json:"namespace"`
	Source      GitopsSource       `json:"source"`
	State       GitopsState        `json:"state"`
	Issues      []GitopsIssue      `json:"issues,omitempty"`
	Resources   []GitopsResource   `json:"resources,omitempty"`
	History     []GitopsDeployment `json:"history,omitempty"`
	Operation   *GitopsOperation   `json:"operation,omitempty"`
	Error       string             `json:"error,omitempty"`
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

const (
	EngineFlux = ControllerFlux
	EngineArgo = ControllerArgo
)

// FleetApp is one delivery object on one cluster, whichever engine manages it.
// Spread is how many clusters carry an app of that name, which is what makes
// the same app on three clusters legible as one thing.
type FleetApp struct {
	Cluster   string `json:"cluster"`
	Engine    string `json:"engine"`
	Kind      string `json:"kind"`
	Group     string `json:"group"`
	Version   string `json:"version"`
	Resource  string `json:"resource"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Ready     string `json:"ready"`
	Sync      string `json:"sync,omitempty"`
	Revision  string `json:"revision,omitempty"`
	Source    string `json:"source,omitempty"`
	Message   string `json:"message,omitempty"`
	Suspended bool   `json:"suspended,omitempty"`
	Spread    int    `json:"spread,omitempty"`
}

type FleetGitops struct {
	Apps  []FleetApp `json:"apps"`
	Error string     `json:"error,omitempty"`
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
	// Only a node has a ceiling, so these stay zero for a pod.
	CPUAllocatableMilli int64 `json:"cpuAllocatableMilli"`
	MemAllocatableMi    int64 `json:"memAllocatableMi"`
}

type UpdateStatus struct {
	Checked   bool   `json:"checked"`
	Current   string `json:"current"`
	Latest    string `json:"latest,omitempty"`
	Available bool   `json:"available"`
	URL       string `json:"url,omitempty"`
	Command   string `json:"command,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// UpdateResult is what pressing the update button came to. Updated means the
// binary on disk was replaced and spinoza has to be restarted; Command is the
// install line for a build that cannot replace itself.
type UpdateResult struct {
	Updated bool   `json:"updated"`
	Current string `json:"current"`
	Latest  string `json:"latest,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Command string `json:"command,omitempty"`
}

type MetricPoint struct {
	At    int64   `json:"at"`
	Value float64 `json:"value"`
}

type MetricHistory struct {
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	Source    string `json:"source,omitempty"`
	Sampled   bool   `json:"sampled,omitempty"`
	// Unix ms of the oldest reading here, not the span asked for.
	Since  int64         `json:"since,omitempty"`
	CPU    []MetricPoint `json:"cpu"`
	Memory []MetricPoint `json:"memory"`
}

type Metrics struct {
	Pods  map[string]ResourceUsage `json:"pods"`
	Nodes map[string]ResourceUsage `json:"nodes"`
	Error string                   `json:"error,omitempty"`
}

type CheckObject struct {
	Cluster   string `json:"cluster,omitempty"`
	Group     string `json:"group"`
	Version   string `json:"version"`
	Resource  string `json:"resource"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Origin    string `json:"origin,omitempty"`
	ManagedBy string `json:"managedBy,omitempty"`
}

type CheckFinding struct {
	Ref       int    `json:"ref"`
	Container string `json:"container,omitempty"`
	Detail    string `json:"detail"`
	Patch     string `json:"patch,omitempty"`
	Severity  string `json:"severity"`
	New       bool   `json:"new,omitempty"`
	Muted     bool   `json:"muted,omitempty"`
	MutedBy   string `json:"mutedBy,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type CheckGroup struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Category   string   `json:"category"`
	Severity   string   `json:"severity"`
	Frameworks []string `json:"frameworks,omitempty"`
	Wrong      string   `json:"wrong"`
	Remedy     string   `json:"remedy"`
	Skipped    string   `json:"skipped,omitempty"`
	Total      int      `json:"total"`
	Muted      int      `json:"muted,omitempty"`
	NewCount   int      `json:"new,omitempty"`
	Fixed      int      `json:"fixed,omitempty"`
	Gone       []string `json:"gone,omitempty"`
	Baselined  bool     `json:"baselined,omitempty"`
	// Was is what this check found when the baseline was taken, and Ran says
	// the baseline ran it at all — nought findings and never asked are not the
	// same thing.
	Was       int            `json:"was,omitempty"`
	Ran       bool           `json:"ran,omitempty"`
	Measured  bool           `json:"measured,omitempty"`
	Truncated bool           `json:"truncated,omitempty"`
	Next      string         `json:"next,omitempty"`
	Findings  []CheckFinding `json:"findings"`
}

type NamespaceCount struct {
	Namespace string `json:"namespace"`
	Total     int    `json:"total"`
	High      int    `json:"high"`
	Medium    int    `json:"medium"`
	Low       int    `json:"low"`
}

type CheckPage struct {
	Findings []CheckFinding `json:"findings"`
	Objects  []CheckObject  `json:"objects"`
	Next     string         `json:"next,omitempty"`
}

type Baseline struct {
	TakenAt  string `json:"takenAt,omitempty"`
	Cluster  string `json:"cluster,omitempty"`
	Findings int    `json:"findings,omitempty"`
	Checks   int    `json:"checks,omitempty"`
}

type Mute struct {
	Check     string `json:"check"`
	Namespace string `json:"namespace,omitempty"`
	Ref       string `json:"ref,omitempty"`
	Reason    string `json:"reason,omitempty"`
	At        string `json:"at,omitempty"`
}

type RuleFault struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

type RuleFaults struct {
	Faults []RuleFault `json:"faults"`
}

type Mutes struct {
	Mutes []Mute `json:"mutes"`
}

type CheckReport struct {
	Groups     []CheckGroup     `json:"groups"`
	Objects    []CheckObject    `json:"objects"`
	Namespaces []NamespaceCount `json:"namespaces,omitempty"`
	Baseline   string           `json:"baseline,omitempty"`
	// BaselineFrom names the cluster a baseline was taken on when that is not
	// this one. Comparing two clusters is a fair thing to want; being told
	// nine thousand findings are new without being told why is not.
	BaselineFrom string `json:"baselineFrom,omitempty"`
	// WasScanned is how many workloads the baseline saw, so a count taken on a
	// cluster of a different size can be read per workload.
	WasScanned int    `json:"wasScanned,omitempty"`
	Scanned    int    `json:"scanned"`
	Error      string `json:"error,omitempty"`
}
