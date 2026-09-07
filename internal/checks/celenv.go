package checks

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"cel.dev/cel-go/cel"
	celast "cel.dev/cel-go/common/ast"
	"cel.dev/cel-go/common/types"
	"cel.dev/cel-go/common/types/ref"
	"cel.dev/cel-go/ext"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	userRuleNow          = "now"
	userRuleCluster      = "cluster"
	clusterListFunction  = "list"
	clusterGetFunction   = "get"
	quantityFunction     = "quantity"
	notAfterFunction     = "x509.notAfter"
	notBeforeFunction    = "x509.notBefore"
	maxCrossObjectCost   = 500_000
	certificateBlockType = "CERTIFICATE"
)

var clusterType = cel.OpaqueType("spinoza.Cluster")

type clusterValue struct {
	held *corpus
}

func (c clusterValue) ConvertToNative(reflect.Type) (any, error) {
	return nil, errors.New("the cluster is not a value a rule can carry out")
}

func (c clusterValue) ConvertToType(kind ref.Type) ref.Val {
	if kind.TypeName() == clusterType.TypeName() {
		return c
	}
	return types.NewErr("the cluster cannot become %s", kind.TypeName())
}

func (c clusterValue) Equal(other ref.Val) ref.Val {
	_, same := other.(clusterValue)
	return types.Bool(same)
}

func (c clusterValue) Type() ref.Type {
	return clusterType
}

func (c clusterValue) Value() any {
	return c.held
}

func userRuleEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable(userRuleObject, cel.DynType),
		cel.Variable(userRuleNow, cel.TimestampType),
		cel.Variable(userRuleCluster, clusterType),
		cel.OptionalTypes(),
		ext.Strings(),
		ext.Lists(),
		ext.Sets(),
		ext.Math(),
		ext.Encoders(),
		ext.TwoVarComprehensions(),
		ext.Regex(),
		cel.Function(
			clusterListFunction,
			cel.MemberOverload("cluster_list",
				[]*cel.Type{clusterType, cel.StringType, cel.StringType}, cel.ListType(cel.DynType),
				cel.FunctionBinding(clusterList)),
		),
		cel.Function(
			clusterGetFunction,
			cel.MemberOverload("cluster_get",
				[]*cel.Type{clusterType, cel.StringType, cel.StringType, cel.StringType, cel.StringType}, cel.DynType,
				cel.FunctionBinding(clusterGet)),
		),
		cel.Function(
			quantityFunction,
			cel.Overload("quantity_string", []*cel.Type{cel.StringType}, cel.DoubleType, cel.UnaryBinding(quantityOf)),
		),
		cel.Function(
			notAfterFunction,
			cel.Overload("x509_notafter_string", []*cel.Type{cel.StringType}, cel.TimestampType, cel.UnaryBinding(notAfterOf)),
		),
		cel.Function(
			notBeforeFunction,
			cel.Overload("x509_notbefore_string", []*cel.Type{cel.StringType}, cel.TimestampType, cel.UnaryBinding(notBeforeOf)),
		),
	)
}

func activationFor(subject Subject, now time.Time, held *corpus) map[string]any {
	return map[string]any{
		userRuleObject:  subject.Object.Object,
		userRuleNow:     now,
		userRuleCluster: clusterValue{held: held},
	}
}

func clusterArgs(args []ref.Val) (*corpus, []string, error) {
	receiver, isCluster := args[0].(clusterValue)
	if !isCluster {
		return nil, nil, errors.New("the receiver is not the cluster")
	}
	words := make([]string, 0, len(args)-1)
	for _, arg := range args[1:] {
		text, isString := arg.(types.String)
		if !isString {
			return nil, nil, errors.New("cluster functions take strings")
		}
		words = append(words, string(text))
	}
	return receiver.held, words, nil
}

func clusterList(args ...ref.Val) ref.Val {
	held, words, err := clusterArgs(args)
	if err != nil {
		return types.NewErr("%s", err.Error())
	}
	group, resourceName := words[0], words[1]
	if !held.read(group, resourceName) {
		return types.NewErr("the audit did not read %s", describeTarget(group, resourceName))
	}
	objects := held.of(group, resourceName)
	out := make([]any, 0, len(objects))
	for _, obj := range objects {
		out = append(out, obj.Object)
	}
	return types.DefaultTypeAdapter.NativeToValue(out)
}

func clusterGet(args ...ref.Val) ref.Val {
	held, words, err := clusterArgs(args)
	if err != nil {
		return types.NewErr("%s", err.Error())
	}
	group, resourceName, namespace, name := words[0], words[1], words[2], words[3]
	if !held.read(group, resourceName) {
		return types.NewErr("the audit did not read %s", describeTarget(group, resourceName))
	}
	for _, obj := range held.of(group, resourceName) {
		if obj.GetNamespace() == namespace && obj.GetName() == name {
			return types.DefaultTypeAdapter.NativeToValue(obj.Object)
		}
	}
	return types.NullValue
}

func describeTarget(group, resourceName string) string {
	if group == "" {
		return resourceName
	}
	return group + "/" + resourceName
}

func quantityOf(value ref.Val) ref.Val {
	text, isString := value.(types.String)
	if !isString {
		return types.NewErr("quantity() takes a string")
	}
	parsed, err := resource.ParseQuantity(string(text))
	if err != nil {
		return types.NewErr("%q is not a quantity: %v", string(text), err)
	}
	return types.Double(parsed.AsApproximateFloat64())
}

func notAfterOf(value ref.Val) ref.Val {
	cert, err := certificateOf(value)
	if err != nil {
		return types.NewErr("%s", err.Error())
	}
	return types.Timestamp{Time: cert.NotAfter}
}

func notBeforeOf(value ref.Val) ref.Val {
	cert, err := certificateOf(value)
	if err != nil {
		return types.NewErr("%s", err.Error())
	}
	return types.Timestamp{Time: cert.NotBefore}
}

func certificateOf(value ref.Val) (*x509.Certificate, error) {
	text, isString := value.(types.String)
	if !isString {
		return nil, errors.New("x509 functions take a string")
	}
	raw := []byte(strings.TrimSpace(string(text)))
	if !strings.HasPrefix(string(raw), "-----BEGIN") {
		decoded, err := base64.StdEncoding.DecodeString(string(raw))
		if err != nil {
			return nil, errors.New("not a PEM certificate, nor base64 of one")
		}
		raw = decoded
	}
	for {
		block, rest := pem.Decode(raw)
		if block == nil {
			return nil, errors.New("no CERTIFICATE block in the PEM")
		}
		if block.Type == certificateBlockType {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("the certificate does not parse: %w", err)
			}
			return cert, nil
		}
		raw = rest
	}
}

func usesCluster(parsed *cel.Ast) bool {
	root := celast.NavigateAST(parsed.NativeRep())
	for _, node := range append(celast.MatchDescendants(root, celast.AllMatcher()), root) {
		if node.Kind() != celast.CallKind {
			continue
		}
		call := node.AsCall()
		if !call.IsMemberFunction() || call.Target().Kind() != celast.IdentKind {
			continue
		}
		if call.Target().AsIdent() == userRuleCluster {
			return true
		}
	}
	return false
}

func costFor(parsed *cel.Ast) uint64 {
	if usesCluster(parsed) {
		return maxCrossObjectCost
	}
	return maxUserRuleCost
}
