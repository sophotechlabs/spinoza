package access

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	authv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/sophotechlabs/spinoza/internal/auth"
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
	remembered          = 30 * time.Second
	atOnce              = 8
	globalReviewLimit   = 32
	identityReviewLimit = 8
	rememberedLimit     = 4096
)

var ErrDenied = errors.New("kubernetes authorization denied")

var ErrUnanswered = errors.New("kubernetes authorization could not be determined")

type answer struct {
	decision Decision
	asked    time.Time
}

type question struct {
	who   string
	check Check
}

type Service struct {
	cs            kubernetes.Interface
	mu            sync.Mutex
	seen          map[question]answer
	inFlight      map[string]int
	now           func() time.Time
	ttl           time.Duration
	capacity      int
	globalSlots   chan struct{}
	identityLimit int
}

func New(cs kubernetes.Interface) *Service {
	return &Service{
		cs:            cs,
		seen:          map[question]answer{},
		inFlight:      map[string]int{},
		now:           time.Now,
		ttl:           remembered,
		capacity:      rememberedLimit,
		globalSlots:   make(chan struct{}, globalReviewLimit),
		identityLimit: identityReviewLimit,
	}
}

func asking(ctx context.Context) string {
	who, ok := auth.ActingAs(ctx)
	if !ok {
		return ""
	}
	var key strings.Builder
	key.WriteString(strconv.Quote(who.User))
	groups := slices.Clone(who.Groups)
	slices.Sort(groups)
	for _, group := range groups {
		key.WriteByte(0)
		key.WriteString(strconv.Quote(group))
	}
	return key.String()
}

func (s *Service) review(ctx context.Context, checks []Check) []Decision {
	out := make([]Decision, len(checks))
	who := asking(ctx)
	pending := map[Check][]int{}
	for i, check := range checks {
		held, ok := s.recall(question{who: who, check: check})
		if ok {
			out[i] = held
			continue
		}
		pending[check] = append(pending[check], i)
	}
	slots := make(chan struct{}, atOnce)
	var group sync.WaitGroup
	for check, places := range pending {
		group.Go(func() {
			slots <- struct{}{}
			defer func() {
				<-slots
			}()
			decision := s.ask(ctx, question{who: who, check: check})
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

func (s *Service) AskFresh(ctx context.Context, check Check) Decision {
	if s == nil {
		return Decision{Allowed: true}
	}
	return s.ask(ctx, question{who: asking(ctx), check: check})
}

func (s *Service) Require(ctx context.Context, checks ...Check) error {
	if s == nil || s.cs == nil {
		return nil
	}
	if _, acting := auth.ActingAs(ctx); !acting {
		return nil
	}
	return require(checks, s.review(ctx, checks))
}

func (s *Service) RequireFresh(ctx context.Context, checks ...Check) error {
	if s == nil || s.cs == nil {
		return nil
	}
	if _, acting := auth.ActingAs(ctx); !acting {
		return nil
	}
	decisions := make([]Decision, 0, len(checks))
	for _, check := range checks {
		decisions = append(decisions, s.AskFresh(ctx, check))
	}
	return require(checks, decisions)
}

func require(checks []Check, decisions []Decision) error {
	for at, decision := range decisions {
		if decision.Allowed && decision.Answered {
			continue
		}
		message := because(decision.Reason, checks[at])
		if !decision.Answered {
			return fmt.Errorf("%w: %s", ErrUnanswered, message)
		}
		return fmt.Errorf("%w: %s", ErrDenied, message)
	}
	return nil
}

func (s *Service) ask(ctx context.Context, want question) Decision {
	if s.cs == nil {
		return Decision{Allowed: true}
	}
	release, ok := s.claim(want.who)
	if !ok {
		return Decision{Reason: "authorization check capacity is full"}
	}
	defer release()
	check := want.check
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
		return Decision{Reason: err.Error()}
	}
	if result.Status.EvaluationError != "" && !result.Status.Allowed {
		return Decision{Reason: result.Status.EvaluationError}
	}
	decision := Decision{Allowed: result.Status.Allowed, Answered: true, Reason: result.Status.Reason}
	if decision.Allowed {
		decision.Reason = ""
	}
	s.remember(want, decision)
	return decision
}

func (s *Service) recall(want question) (Decision, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	held, ok := s.seen[want]
	if !ok {
		return Decision{}, false
	}
	if s.now().Sub(held.asked) > s.ttl {
		delete(s.seen, want)
		return Decision{}, false
	}
	return held.decision, true
}

func (s *Service) remember(want question, decision Decision) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for key, held := range s.seen {
		if now.Sub(held.asked) > s.ttl {
			delete(s.seen, key)
		}
	}
	capacity := s.capacity
	if capacity <= 0 {
		capacity = rememberedLimit
	}
	if _, exists := s.seen[want]; !exists && len(s.seen) >= capacity {
		var oldest question
		var oldestAt time.Time
		for key, held := range s.seen {
			if oldestAt.IsZero() || held.asked.Before(oldestAt) {
				oldest = key
				oldestAt = held.asked
			}
		}
		delete(s.seen, oldest)
	}
	s.seen[want] = answer{decision: decision, asked: now}
}

func (s *Service) claim(identity string) (func(), bool) {
	select {
	case s.globalSlots <- struct{}{}:
	default:
		return nil, false
	}
	s.mu.Lock()
	limit := s.identityLimit
	if limit <= 0 {
		limit = identityReviewLimit
	}
	if s.inFlight[identity] >= limit {
		s.mu.Unlock()
		<-s.globalSlots
		return nil, false
	}
	s.inFlight[identity]++
	s.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			s.inFlight[identity]--
			if s.inFlight[identity] == 0 {
				delete(s.inFlight, identity)
			}
			s.mu.Unlock()
			<-s.globalSlots
		})
	}, true
}
