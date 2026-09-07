package checks

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func ruled(t *testing.T, rules []UserRule, objects ...*unstructured.Unstructured) api.CheckReport {
	t.Helper()
	keep := wholeCluster()
	keep.Rules = rules
	return Run(t.Context(), newLister(objects...), descriptors(), api.Metrics{}, keep, 0)
}

func judging(id, expr string) UserRule {
	return UserRule{ID: id, Expr: expr}
}

func typedService(name, kind string, selector map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   metadata(name),
		"spec":       map[string]any{"type": kind, "selector": selector},
	}}
}

func withTemplateLabels(obj *unstructured.Unstructured, labels map[string]any) *unstructured.Unstructured {
	template, ok := specOf(obj)["template"].(map[string]any)
	if !ok {
		return obj
	}
	template["metadata"] = map[string]any{"labels": labels}
	return obj
}

func certificatePEM(t *testing.T, notAfter time.Time) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "api.example"},
		NotBefore:    notAfter.Add(-90 * 24 * time.Hour),
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// the libraries a rule may lean on

func TestARuleMayUseTheExtensionLibraries(t *testing.T) {
	cases := []struct {
		name string
		expr string
	}{
		{name: "strings", expr: `object.metadata.name.upperAscii() == 'API' && 'a,b'.split(',').size() == 2`},
		{name: "lists", expr: `[3, 1, 3].distinct().size() == 2 && [[1], [2]].flatten() == [1, 2]`},
		{name: "sets", expr: `sets.contains([1, 2, 3], [2]) && !sets.intersects([1], [2])`},
		{name: "math", expr: `math.greatest(1, 5, 3) == 5 && math.least(4.0, 2.0) == 2.0`},
		{name: "encoders", expr: `base64.decode('YXBp') == b'api' && base64.encode(b'api') == 'YXBp'`},
		{name: "two-variable comprehensions", expr: `[10, 20].all(i, v, v > i) && {'a': 1}.transformMap(k, v, v + 1) == {'a': 2}`},
		{name: "regex", expr: `regex.extract(object.metadata.name, 'a(p)i') == optional.of('p')`},
		{name: "the time of the audit", expr: `now > timestamp('2020-01-01T00:00:00Z')`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			found := ruled(t, []UserRule{judging("lib", tc.expr)}, deployment("api", podSpec(container("app", nil))))
			if found.Error != "" {
				t.Fatalf("error = %q", found.Error)
			}
			if findingCount(t, found, "lib") != 1 {
				t.Fatalf("%s did not hold on the deployment", tc.expr)
			}
		})
	}
}

func TestARuleCanCompareQuantities(t *testing.T) {
	rule := judging("fat", `object.spec.template.spec.containers.exists(c, quantity(c.resources.limits.memory) > quantity('1Gi'))`)
	fat := deployment("fat", podSpec(container("app", resources("limits", map[string]any{"memory": "2Gi"}))))
	thin := deployment("thin", podSpec(container("app", resources("limits", map[string]any{"memory": "512Mi"}))))

	found := ruled(t, []UserRule{rule}, fat, thin)

	if onlyObject(t, found, "fat").Name != "fat" {
		t.Fatal("2Gi was not judged larger than 1Gi, or 512Mi was")
	}
}

func TestAQuantityNobodyCanParseIsAFaultTheReportNames(t *testing.T) {
	rule := judging("fat", `quantity(object.spec.template.spec.containers[0].resources.limits.memory) > quantity('1Gi')`)
	odd := deployment("odd", podSpec(container("app", resources("limits", map[string]any{"memory": "lots"}))))

	found := ruled(t, []UserRule{rule}, odd)

	want := `rule "fat" could not evaluate for Deployment apps/odd: "lots" is not a quantity`
	if !strings.Contains(found.Error, want) {
		t.Fatalf("error = %q, want %q", found.Error, want)
	}
	if findingCount(t, found, "fat") != 0 {
		t.Fatal("a rule that could not evaluate still produced a finding")
	}
}

func TestARuleCanReadACertificatesExpiry(t *testing.T) {
	soon := certificatePEM(t, time.Now().Add(10*24*time.Hour))
	later := certificatePEM(t, time.Now().Add(200*24*time.Hour))
	rule := judging("expiring", `x509.notAfter(cluster.get('', 'configmaps', 'apps', object.metadata.name).data['tls.crt']) < now + duration('720h')`)

	found := ruled(t, []UserRule{rule},
		deployment("soon", podSpec(container("app", nil))),
		configMap("soon", map[string]any{"tls.crt": soon}),
		deployment("later", podSpec(container("app", nil))),
		configMap("later", map[string]any{"tls.crt": base64.StdEncoding.EncodeToString([]byte(later))}))

	if found.Error != "" {
		t.Fatalf("error = %q", found.Error)
	}
	if onlyObject(t, found, "expiring").Name != "soon" {
		t.Fatal("the certificate expiring in ten days was not the one reported")
	}
}

func TestACertificateNobodyCanReadIsAFault(t *testing.T) {
	cases := []struct {
		name string
		data string
		want string
	}{
		{name: "not base64", data: "not a certificate", want: "not a PEM certificate, nor base64 of one"},
		{name: "pem without a certificate", data: "-----BEGIN KEY-----\nAA==\n-----END KEY-----\n", want: "no CERTIFICATE block in the PEM"},
		{name: "garbage in the block", data: "-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n", want: "the certificate does not parse"},
	}
	for at, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			function := notBeforeFunction
			if at%2 == 1 {
				function = notAfterFunction
			}
			rule := judging("cert", function+`(cluster.get('', 'configmaps', 'apps', 'tls').data['tls.crt']) < now`)
			found := ruled(t, []UserRule{rule},
				deployment("api", podSpec(container("app", nil))),
				configMap("tls", map[string]any{"tls.crt": tc.data}))
			if !strings.Contains(found.Error, `rule "cert" could not evaluate for Deployment apps/api: `+tc.want) {
				t.Fatalf("error = %q, want %q", found.Error, tc.want)
			}
		})
	}
}

// reaching the rest of the cluster

func TestARuleCanJudgeAServiceAgainstTheDeploymentsItSelects(t *testing.T) {
	rule := UserRule{
		ID:    "lb-selects-nothing",
		Match: "Service",
		Expr: `object.spec.type == 'LoadBalancer' && !cluster.list('apps', 'deployments').exists(d,` +
			` d.metadata.namespace == object.metadata.namespace &&` +
			` object.spec.selector.all(k, k in d.spec.template.metadata.labels && d.spec.template.metadata.labels[k] == object.spec.selector[k]))`,
	}

	found := ruled(t, []UserRule{rule},
		withTemplateLabels(deployment("api", podSpec(container("app", nil))), map[string]any{"app": "api"}),
		typedService("api", "LoadBalancer", map[string]any{"app": "api"}),
		typedService("orphan", "LoadBalancer", map[string]any{"app": "gone"}),
		typedService("internal", "ClusterIP", map[string]any{"app": "gone"}))

	if found.Error != "" {
		t.Fatalf("error = %q", found.Error)
	}
	object := onlyObject(t, found, "lb-selects-nothing")
	if object.Kind != "Service" || object.Name != "orphan" || object.Resource != "services" {
		t.Fatalf("object = %+v, want the orphaned load balancer", object)
	}
}

func TestARuleWithNoMatchStaysOnTheWorkloads(t *testing.T) {
	found := ruled(t, []UserRule{judging("everything", `true`)},
		deployment("api", podSpec(container("app", nil))),
		typedService("api", "ClusterIP", map[string]any{"app": "api"}))

	if onlyObject(t, found, "everything").Kind != "Deployment" {
		t.Fatal("a rule that names no kind was judged against a Service")
	}
}

func TestAKindTheAuditDidNotReadIsAFaultNotAnEmptyList(t *testing.T) {
	rule := judging("certs", `cluster.list('cert-manager.io', 'certificates').size() == 0`)

	found := ruled(t, []UserRule{rule}, deployment("api", podSpec(container("app", nil))))

	want := `rule "certs" could not evaluate for Deployment apps/api: the audit did not read cert-manager.io/certificates`
	if !strings.Contains(found.Error, want) {
		t.Fatalf("error = %q, want %q", found.Error, want)
	}
	if findingCount(t, found, "certs") != 0 {
		t.Fatal("an unread kind passed as an empty list")
	}
}

func TestAKindThatWasReadAndIsEmptyIsAnEmptyList(t *testing.T) {
	rule := judging("no-services", `cluster.list('', 'services').size() == 0`)

	found := ruled(t, []UserRule{rule}, deployment("api", podSpec(container("app", nil))))

	if found.Error != "" || findingCount(t, found, "no-services") != 1 {
		t.Fatalf("error = %q, findings = %d; want a read but empty kind to be an empty list", found.Error, findingCount(t, found, "no-services"))
	}
}

func TestAnObjectTheAuditDidNotFindIsNull(t *testing.T) {
	rule := judging("no-config", `cluster.get('', 'configmaps', object.metadata.namespace, object.metadata.name) == null`)

	found := ruled(t, []UserRule{rule},
		deployment("api", podSpec(container("app", nil))),
		deployment("web", podSpec(container("app", nil))),
		configMap("web", map[string]any{"a": "b"}))

	if found.Error != "" || onlyObject(t, found, "no-config").Name != "api" {
		t.Fatalf("error = %q; want only the deployment without a config map", found.Error)
	}
}

func TestClusterFunctionsRefuseTheWrongArguments(t *testing.T) {
	cases := []struct {
		name string
		expr string
		want string
	}{
		{name: "list with too few", expr: `cluster.list('apps').size() == 0`, want: "did not compile"},
		{name: "get with a number", expr: `cluster.get('', 'configmaps', 'apps', 1) == null`, want: "did not compile"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			faults := Faults(`[{"id":"x","expr":` + strconv.Quote(tc.expr) + `}]`)
			if len(faults) != 1 || !strings.Contains(faults[0].Reason, tc.want) {
				t.Fatalf("faults = %+v, want %q", faults, tc.want)
			}
		})
	}
}

func TestACrossObjectRuleGetsTheBudgetItNeeds(t *testing.T) {
	objects := manyDeployments(300)
	objects = append(objects, typedService("api", "LoadBalancer", map[string]any{"app": "nothing"}))
	rule := UserRule{
		ID:    "lb-selects-nothing",
		Match: "Service",
		Expr: `!cluster.list('apps', 'deployments').exists(d, has(d.metadata.labels) &&` +
			` object.spec.selector.all(k, k in d.metadata.labels && d.metadata.labels[k] == object.spec.selector[k]))`,
	}

	found := ruled(t, []UserRule{rule}, objects...)

	if found.Error != "" {
		t.Fatalf("error = %q, want a rule over 300 deployments to fit its budget", found.Error)
	}
	if findingCount(t, found, "lb-selects-nothing") != 1 {
		t.Fatal("the load balancer selecting nothing was not reported")
	}
}

func TestARuleWithoutClusterFunctionsKeepsTheSmallBudget(t *testing.T) {
	if costFor(mustParse(t, `true`)) != maxUserRuleCost {
		t.Fatal("a plain rule was given the cross-object budget")
	}
	if costFor(mustParse(t, `cluster.list('', 'services').size() > 0`)) != maxCrossObjectCost {
		t.Fatal("a rule reaching the cluster kept the small budget")
	}
}

func mustParse(t *testing.T, expr string) *cel.Ast {
	t.Helper()
	parsed, err := parseRule(expr)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestTheValidatorKnowsTheSameFunctions(t *testing.T) {
	faults := Faults(`[{"id":"x","expr":"quantity('1') > 0.0 && cluster.list('', 'pods').size() >= 0 && x509.notAfter('') > now"}]`)
	if len(faults) != 0 {
		t.Fatalf("faults = %+v, want the functions to compile in validation", faults)
	}
}

func TestASilencerMayReachTheClusterToo(t *testing.T) {
	rules := []UserRule{{
		ID:       "quiet-configured",
		Silences: "privileged-containers",
		Reason:   "the config map says so",
		Expr:     `cluster.get('', 'configmaps', object.metadata.namespace, 'allow') != null`,
	}}
	keep := wholeCluster()
	keep.Rules = rules
	keep.Silencers = Silencers(rules)
	keep.ShowMuted = true
	found := Run(t.Context(), newLister(
		deployment("api", podSpec(container("app", withSecurity(map[string]any{"privileged": true})))),
		configMap("allow", map[string]any{"a": "b"}),
	), descriptors(), api.Metrics{}, keep, 0)

	finding := onlyFinding(t, found, "privileged-containers")
	if !finding.Muted || finding.Reason != "the config map says so" {
		t.Fatalf("finding = %+v, want it silenced by the rule that read the config map", finding)
	}
}

// the cluster value and the bindings, called the way the evaluator calls them

func TestTheClusterValueBehavesAsACELValue(t *testing.T) {
	held := newCorpus(nil, nil, nil, nil, nil, nil)
	value := clusterValue{held: held}
	if value.Type().TypeName() != "spinoza.Cluster" || value.Value() != held {
		t.Fatal("the cluster value does not name its type or carry its corpus")
	}
	if value.ConvertToType(clusterType) != value {
		t.Fatal("converting the cluster to its own type changed it")
	}
	if err := value.ConvertToType(types.StringType); !types.IsError(err) {
		t.Fatalf("converting the cluster to a string gave %v, want an error", err)
	}
	if value.Equal(clusterValue{held: held}) != types.True || value.Equal(types.String("x")) != types.False {
		t.Fatal("cluster equality does not go by kind")
	}
	if _, err := value.ConvertToNative(reflect.TypeFor[string]()); err == nil {
		t.Fatal("the cluster became a native value")
	}
}

func TestTheClusterBindingsRefuseWhatTheCheckerNeverSends(t *testing.T) {
	held := newCorpus(nil, nil, nil, nil, nil, nil)
	if got := clusterList(types.String("not the cluster"), types.String(""), types.String("pods")); !types.IsError(got) {
		t.Fatalf("list without the cluster receiver gave %v", got)
	}
	if got := clusterGet(clusterValue{held: held}, types.Int(1), types.String("pods"), types.String(""), types.String("x")); !types.IsError(got) {
		t.Fatalf("get with a number gave %v", got)
	}
	if got := quantityOf(types.Int(1)); !types.IsError(got) {
		t.Fatalf("quantity of a number gave %v", got)
	}
	if got := notAfterOf(types.Int(1)); !types.IsError(got) {
		t.Fatalf("notAfter of a number gave %v", got)
	}
	if got := notBeforeOf(types.Int(1)); !types.IsError(got) {
		t.Fatalf("notBefore of a number gave %v", got)
	}
}

func TestACoreKindTheAuditDidNotReadIsNamedWithoutAGroup(t *testing.T) {
	found := ruled(t, []UserRule{judging("endpoints", `cluster.list('', 'endpoints').size() == 0`)},
		deployment("api", podSpec(container("app", nil))))

	if !strings.Contains(found.Error, "the audit did not read endpoints") {
		t.Fatalf("error = %q", found.Error)
	}
}

func TestAnExpressionPastTheLengthLimitDoesNotCompile(t *testing.T) {
	if _, err := compileRule(strings.Repeat("true && ", 1000) + "true"); err == nil || !strings.Contains(err.Error(), "cannot be longer") {
		t.Fatalf("err = %v", err)
	}
}
