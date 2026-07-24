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
	Name string `json:"name"`
}

type Row struct {
	UID       string   `json:"uid"`
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	CreatedAt string   `json:"createdAt"`
	Cells     []string `json:"cells"`
}

type ClientMsg struct {
	Type      string `json:"type"`
	SubID     string `json:"subId"`
	Group     string `json:"group"`
	Version   string `json:"version"`
	Resource  string `json:"resource"`
	Namespace string `json:"namespace"`
}

type ServerMsg struct {
	Type       string   `json:"type"`
	SubID      string   `json:"subId,omitempty"`
	Columns    []Column `json:"columns,omitempty"`
	Namespaced bool     `json:"namespaced,omitempty"`
	Rows       []Row    `json:"rows,omitempty"`
	Row        *Row     `json:"row,omitempty"`
	UID        string   `json:"uid,omitempty"`
	Message    string   `json:"message,omitempty"`
}

type GraphNode struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Group     string `json:"group"`
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
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Ready     string `json:"ready"`
	Suspended bool   `json:"suspended"`
	Revision  string `json:"revision"`
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
