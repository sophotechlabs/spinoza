// Package rbac answers the question the apiserver will not: not "may I do
// this", which SelfSubjectAccessReview already covers, but "who may do this",
// which needs the bindings read and turned inside out.
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

// Subject is who a rule reaches. A service account is the only kind that has a
// namespace of its own; a user or group is a name the authenticator supplies.
type Subject struct {
	Kind      string
	Name      string
	Namespace string
}

func (s Subject) Key() string {
	return s.Kind + "/" + s.Namespace + "/" + s.Name
}

// Label is how a person refers to the subject: a service account by the name
// the apiserver uses for it, anything else by its own name.
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

// Grant is one binding's contribution to a subject: which role, through which
// binding, and where it applies. Namespace is empty for a grant that applies
// everywhere.
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

// Holder is a subject and everything bound to it.
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
