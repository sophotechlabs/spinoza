package access

import (
	"context"
	"sync"
	"time"

	authv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Check is one question for the apiserver: may this user do this verb to this
// resource here. It is also the cache key, so every field is part of the ask.
type Check struct {
	Verb        string
	Group       string
	Resource    string
	Subresource string
	Namespace   string
	Name        string
}

// Decision is what the cluster answered. A check that could not be asked comes
// back allowed with no reason: spinoza never takes away a button over a question
// it failed to put.
type Decision struct {
	Allowed bool
	Reason  string
}

const (
	remembered = 30 * time.Second
	atOnce     = 8
)

type answer struct {
	decision Decision
	asked    time.Time
}

type Service struct {
	cs   kubernetes.Interface
	mu   sync.Mutex
	seen map[Check]answer
	now  func() time.Time
	ttl  time.Duration
}

func New(cs kubernetes.Interface) *Service {
	return &Service{cs: cs, seen: map[Check]answer{}, now: time.Now, ttl: remembered}
}

// review answers every check, asking the apiserver only about the ones whose
// answer is not still fresh. The same question asked twice in one pass is one
// question: a selection of fifty nodes shares the cluster-wide read a drain
// needs, and the cache cannot help with answers that have not arrived yet.
func (s *Service) review(ctx context.Context, checks []Check) []Decision {
	out := make([]Decision, len(checks))
	asking := map[Check][]int{}
	for i, check := range checks {
		held, ok := s.recall(check)
		if ok {
			out[i] = held
			continue
		}
		asking[check] = append(asking[check], i)
	}
	slots := make(chan struct{}, atOnce)
	var group sync.WaitGroup
	for check, places := range asking {
		group.Go(func() {
			slots <- struct{}{}
			defer func() {
				<-slots
			}()
			decision := s.ask(ctx, check)
			for _, at := range places {
				out[at] = decision
			}
		})
	}
	group.Wait()
	return out
}

func (s *Service) ask(ctx context.Context, check Check) Decision {
	if s.cs == nil {
		return Decision{Allowed: true}
	}
	review := &authv1.SelfSubjectAccessReview{
		Spec: authv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authv1.ResourceAttributes{
				Namespace:   check.Namespace,
				Verb:        check.Verb,
				Group:       check.Group,
				Resource:    check.Resource,
				Subresource: check.Subresource,
				Name:        check.Name,
			},
		},
	}
	result, err := s.cs.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, review, metav1.CreateOptions{})
	if err != nil {
		return Decision{Allowed: true}
	}
	// An authorizer that could not make up its mind is not a refusal either, and
	// an answer that shaky is not worth remembering.
	if result.Status.EvaluationError != "" && !result.Status.Allowed {
		return Decision{Allowed: true}
	}
	decision := Decision{Allowed: result.Status.Allowed, Reason: result.Status.Reason}
	if decision.Allowed {
		decision.Reason = ""
	}
	s.remember(check, decision)
	return decision
}

func (s *Service) recall(check Check) (Decision, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	held, ok := s.seen[check]
	if !ok {
		return Decision{}, false
	}
	if s.now().Sub(held.asked) > s.ttl {
		delete(s.seen, check)
		return Decision{}, false
	}
	return held.decision, true
}

func (s *Service) remember(check Check, decision Decision) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen[check] = answer{decision: decision, asked: s.now()}
}
