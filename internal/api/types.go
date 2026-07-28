package api

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

type Column struct {
	Name   string `json:"name"`
	Render string `json:"render,omitempty"`
}

type ContainerState struct {
	Name     string `json:"name"`
	State    string `json:"state"`
	Reason   string `json:"reason,omitempty"`
	Ready    bool   `json:"ready"`
	Restarts int64  `json:"restarts"`
	Init     bool   `json:"init"`
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
	HandledAt   string            `json:"handledAt,omitempty"`
	Ports       []ObjectPort      `json:"ports,omitempty"`
	YAML        string            `json:"yaml"`
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
	ShellUnknown = "unknown"
	ShellPresent = "present"
	ShellAbsent  = "absent"
)

type FluxActionResult struct {
	Action      string `json:"action"`
	RequestedAt string `json:"requestedAt,omitempty"`
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
}

type ServerMsg struct {
	Type       string   `json:"type"`
	SubID      string   `json:"subId,omitempty"`
	Columns    []Column `json:"columns,omitempty"`
	Namespaced bool     `json:"namespaced,omitempty"`
	Rows       []Row    `json:"rows,omitempty"`
	Row        *Row     `json:"row,omitempty"`
	UID        string   `json:"uid,omitempty"`
	Lines      []string `json:"lines,omitempty"`
	Message    string   `json:"message,omitempty"`
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
	Total     int            `json:"total"`
	Resources []FluxResource `json:"resources"`
}

type FluxDashboard struct {
	Groups []FluxGroup `json:"groups"`
}

type ResourceUsage struct {
	CPUMilli   int64 `json:"cpuMilli"`
	MemoryMi   int64 `json:"memoryMi"`
	CPUPercent int64 `json:"cpuPercent"`
	MemPercent int64 `json:"memPercent"`
}

type Metrics struct {
	Pods  map[string]ResourceUsage `json:"pods"`
	Nodes map[string]ResourceUsage `json:"nodes"`
}
