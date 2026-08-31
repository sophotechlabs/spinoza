package access

import (
	"context"
	"sync"
	"time"

	authv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Check struct {
	Verb        string
	Group       string
	Resource    string
	Subresource string
	Namespace   string
	Name        string
}

type Decision struct {
	Allowed  bool
	Answered bool
	Reason   string
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

func (s *Service) Ask(ctx context.Context, check Check) Decision {
	if s == nil {
		return Decision{Allowed: true}
	}
	return s.review(ctx, []Check{check})[0]
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
		return Decision{Allowed: true, Reason: err.Error()}
	}
	if result.Status.EvaluationError != "" && !result.Status.Allowed {
		return Decision{Allowed: true, Reason: result.Status.EvaluationError}
	}
	decision := Decision{Allowed: result.Status.Allowed, Answered: true, Reason: result.Status.Reason}
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
