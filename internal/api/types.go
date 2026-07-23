package api

type PodRow struct {
	UID       string `json:"uid"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Phase     string `json:"phase"`
	Ready     string `json:"ready"`
	Restarts  int32  `json:"restarts"`
	Node      string `json:"node"`
	CreatedAt string `json:"createdAt"`
}

type ServerMsg struct {
	Type     string   `json:"type"`
	Resource string   `json:"resource,omitempty"`
	Items    []PodRow `json:"items,omitempty"`
	Item     *PodRow  `json:"item,omitempty"`
	UID      string   `json:"uid,omitempty"`
	RV       string   `json:"rv,omitempty"`
	Message  string   `json:"message,omitempty"`
}
