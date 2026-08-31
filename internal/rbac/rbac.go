package rbac

const Anything = "*"

const (
	KindUser           = "User"
	KindGroup          = "Group"
	KindServiceAccount = "ServiceAccount"
)

const (
	RoleKind        = "Role"
	ClusterRoleKind = "ClusterRole"
)

type Subject struct {
	Kind      string
	Name      string
	Namespace string
}

func (s Subject) Key() string {
	return s.Kind + "/" + s.Namespace + "/" + s.Name
}

func (s Subject) Label() string {
	if s.Kind != KindServiceAccount {
		return s.Name
	}
	if s.Namespace == "" {
		return s.Name
	}
	return "system:serviceaccount:" + s.Namespace + ":" + s.Name
}

type Rule struct {
	Verbs     []string
	Groups    []string
	Resources []string
	Names     []string
	URLs      []string
}

type Grant struct {
	Binding     string
	BindingKind string
	Role        string
	RoleKind    string
	Namespace   string
	Rules       []Rule
	Missing     bool
	Aggregated  bool
	RuleCount   int
}

func (g Grant) Everywhere() bool {
	return g.Namespace == ""
}

type Holder struct {
	Subject Subject
	Grants  []Grant
	Powers  []string
}

type Index struct {
	Holders []Holder
	Absent  []string
	Error   string
}
