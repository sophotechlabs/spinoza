package checks

import (
	"slices"
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	psapi "k8s.io/pod-security-admission/api"
	"k8s.io/pod-security-admission/policy"
)

type upstreamControl struct {
	id    policy.CheckID
	level psapi.Level
	from  psapi.Version
	alone policy.Evaluator
}

type upstreamPolicy struct {
	controls map[policy.CheckID]upstreamControl
	whole    policy.Evaluator
}

func upstreamFrom(checks []policy.Check) (upstreamPolicy, error) {
	whole, wholeErr := policy.NewEvaluator(checks, nil)
	if wholeErr != nil {
		return upstreamPolicy{}, wholeErr
	}
	out := upstreamPolicy{controls: map[policy.CheckID]upstreamControl{}, whole: whole}
	for _, one := range checks {
		alone, aloneErr := policy.NewEvaluator([]policy.Check{one}, nil)
		if aloneErr != nil {
			return upstreamPolicy{}, aloneErr
		}
		out.controls[one.ID] = upstreamControl{
			id:    one.ID,
			level: one.Level,
			from:  one.Versions[0].MinimumVersion,
			alone: alone,
		}
	}
	return out, nil
}

var upstream = sync.OnceValues(func() (upstreamPolicy, error) {
	return upstreamFrom(policy.DefaultChecks())
})

func upstreamVersionOf(serverVersion string) psapi.Version {
	minor := minorOf(serverVersion)
	if minor == 0 {
		return psapi.LatestVersion()
	}
	return psapi.MajorMinorVersion(1, minor)
}

type upstreamPod struct {
	meta *metav1.ObjectMeta
	spec *corev1.PodSpec
	err  error
}

type upstreamScan struct {
	version psapi.Version
	policy  upstreamPolicy
	err     error
	mu      sync.Mutex
	pods    map[string]upstreamPod
}

func newUpstreamScan(serverVersion string) *upstreamScan {
	loaded, err := upstream()
	return &upstreamScan{
		version: upstreamVersionOf(serverVersion),
		policy:  loaded,
		err:     err,
		pods:    map[string]upstreamPod{},
	}
}

func (u *upstreamScan) podOf(subject Subject) upstreamPod {
	key := subjectKey(subject)
	u.mu.Lock()
	defer u.mu.Unlock()
	if pod, seen := u.pods[key]; seen {
		return pod
	}
	pod := decodePod(subject)
	u.pods[key] = pod
	return pod
}

func decodePod(subject Subject) upstreamPod {
	spec := &corev1.PodSpec{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(subject.Pod, spec); err != nil {
		return upstreamPod{err: err}
	}
	meta := &metav1.ObjectMeta{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(templateMetaOf(subject), meta); err != nil {
		return upstreamPod{err: err}
	}
	return upstreamPod{meta: meta, spec: spec}
}

func templateMetaOf(subject Subject) map[string]any {
	return specAt(subject.Object, templateMetaPath(subject.Kind)...)
}

func templateMetaPath(kind string) []string {
	if kind == "Pod" {
		return []string{metadataField}
	}
	if kind == cronKind {
		return []string{specField, "jobTemplate", specField, "template", metadataField}
	}
	return []string{specField, "template", metadataField}
}

const upstreamUnloaded = "not audited: the upstream Pod Security policy could not be loaded: "

func (u *upstreamScan) standsDown(id policy.CheckID) string {
	if u.err != nil {
		return upstreamUnloaded + u.err.Error()
	}
	control, known := u.policy.controls[id]
	if !known {
		return "not audited: upstream has no control named " + string(id)
	}
	if u.version.Older(control.from) {
		return "not audited: upstream applies this from " + control.from.String() +
			" and this cluster reports " + u.version.String()
	}
	return ""
}

func (u *upstreamScan) verdict(id policy.CheckID, subject Subject) (policy.CheckResult, bool) {
	if u.standsDown(id) != "" {
		return policy.CheckResult{}, false
	}
	pod := u.podOf(subject)
	if pod.err != nil {
		return policy.CheckResult{}, false
	}
	control := u.policy.controls[id]
	asked := psapi.LevelVersion{Level: control.level, Version: u.version}
	results := control.alone.EvaluatePod(asked, pod.meta, pod.spec)
	if len(results) != 1 {
		return policy.CheckResult{}, false
	}
	return results[0], true
}

func (u *upstreamScan) enforcedOn(labels map[string]string) (psapi.LevelVersion, bool) {
	open := psapi.LevelVersion{Level: psapi.LevelPrivileged, Version: psapi.LatestVersion()}
	read, errs := psapi.PolicyToEvaluate(labels, psapi.Policy{Enforce: open, Audit: open, Warn: open})
	return read.Enforce, len(errs) == 0
}

func (u *upstreamScan) rejects(enforced psapi.LevelVersion, subject Subject) string {
	if u.err != nil {
		return ""
	}
	pod := u.podOf(subject)
	if pod.err != nil {
		return ""
	}
	total := policy.AggregateCheckResults(u.policy.whole.EvaluatePod(enforced, pod.meta, pod.spec))
	if total.Allowed {
		return ""
	}
	return total.ForbiddenReason()
}

func upstreamDetail(result policy.CheckResult) string {
	if result.ForbiddenDetail == "" {
		return result.ForbiddenReason
	}
	return result.ForbiddenReason + " (" + result.ForbiddenDetail + ")"
}

func upstreamOnly(scan) []found {
	return nil
}

func (c check) findings(sc scan) []found {
	if c.upstream == "" {
		return c.find(sc)
	}
	return c.decidedUpstream(sc)
}

func (c check) decidedUpstream(sc scan) []found {
	bySubject := map[string][]found{}
	for _, item := range c.find(sc) {
		key := subjectKey(item.subject)
		bySubject[key] = append(bySubject[key], item)
	}
	out := []found{}
	for _, subject := range sc.subjects {
		key := subjectKey(subject)
		result, decided := sc.upstream.verdict(c.upstream, subject)
		if !decided {
			out = append(out, bySubject[key]...)
			continue
		}
		if result.Allowed {
			continue
		}
		if len(bySubject[key]) > 0 || c.presentedBy(subject) {
			out = append(out, bySubject[key]...)
			continue
		}
		out = append(out, found{subject: subject, detail: upstreamDetail(result)})
	}
	return out
}

func (c check) presentedBy(subject Subject) bool {
	if c.presented == nil {
		return false
	}
	return c.presented(subject)
}

func pssLabelOf(level psapi.Level) string {
	if level == psapi.LevelRestricted {
		return pssRestricted
	}
	return pssBaseline
}

func (c check) frameworksFor(sc scan) []string {
	if c.upstream == "" {
		return c.frameworks
	}
	if sc.upstream.standsDown(c.upstream) != "" {
		return c.frameworks
	}
	control := sc.upstream.policy.controls[c.upstream]
	out := []string{pssLabelOf(control.level)}
	return slices.Concat(out, c.frameworks)
}

func enforcedLevelRejects(sc scan) []found {
	out := []found{}
	for _, subject := range sc.subjects {
		detail := rejectedByEnforcedLevel(sc, subject)
		if detail == "" {
			continue
		}
		out = append(out, found{subject: subject, detail: detail})
	}
	return out
}

func rejectedByEnforcedLevel(sc scan, subject Subject) string {
	space := sc.held.namespace(subject.Ref.Namespace)
	if space == nil {
		return ""
	}
	enforced, readable := sc.upstream.enforcedOn(space.GetLabels())
	if enforced.Level == psapi.LevelPrivileged {
		return ""
	}
	broken := sc.upstream.rejects(enforced, subject)
	if broken == "" {
		return ""
	}
	return "the namespace enforces " + enforcedWording(enforced, readable) + " and this breaks " + broken
}

func enforcedWording(enforced psapi.LevelVersion, readable bool) string {
	out := string(enforced.Level)
	if !enforced.Latest() {
		out += " as of " + enforced.Version.String()
	}
	if !readable {
		out += ", which is what upstream falls back to for a label it cannot read,"
	}
	return out
}
