package prom

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const (
	rangePath   = "api/v1/query_range"
	buildPath   = "api/v1/status/buildinfo"
	promNameKey = "app.kubernetes.io/name"
	promNameVal = "prometheus"
)

var selectors = []string{
	"operated-prometheus=true",
	promNameKey + "=" + promNameVal,
}

var ErrUnavailable = errors.New("prometheus is unavailable")

type Target struct {
	Namespace string
	Service   string
	Port      string
	Scheme    string
}

func (t Target) String() string {
	return fmt.Sprintf("%s/%s:%s (%s)", t.Namespace, t.Service, t.Port, t.Scheme)
}

type Proxy interface {
	Get(ctx context.Context, target Target, path string, params map[string]string) ([]byte, error)
}

type serviceProxy struct {
	cs kubernetes.Interface
}

func (s *serviceProxy) Get(ctx context.Context, target Target, path string, params map[string]string) ([]byte, error) {
	return s.cs.CoreV1().Services(target.Namespace).
		ProxyGet(target.Scheme, target.Service, target.Port, path, params).
		DoRaw(ctx)
}

type Client struct {
	cs       kubernetes.Interface
	proxy    Proxy
	override Target
	mu       sync.Mutex
	resolved *Target
}

func NewClient(cs kubernetes.Interface, override Target) *Client {
	return &Client{cs: cs, proxy: &serviceProxy{cs: cs}, override: override}
}

func newClientWithProxy(cs kubernetes.Interface, proxy Proxy, override Target) *Client {
	return &Client{cs: cs, proxy: proxy, override: override}
}

func ParseTarget(spec string) (Target, error) {
	if spec == "" {
		return Target{}, nil
	}
	namespace, rest, found := strings.Cut(spec, "/")
	if !found {
		return Target{}, fmt.Errorf("expected namespace/service:port, got %q", spec)
	}
	service, port, hasPort := strings.Cut(rest, ":")
	if !hasPort {
		port = "9090"
	}
	if namespace == "" || service == "" {
		return Target{}, fmt.Errorf("expected namespace/service:port, got %q", spec)
	}
	return Target{Namespace: namespace, Service: service, Port: port}, nil
}

func (c *Client) Target(ctx context.Context) (Target, error) {
	c.mu.Lock()
	cached := c.resolved
	c.mu.Unlock()
	if cached != nil {
		return *cached, nil
	}

	target, err := c.discover(ctx)
	if err != nil {
		return Target{}, err
	}
	scheme, err := c.probe(ctx, target)
	if err != nil {
		return Target{}, err
	}
	target.Scheme = scheme

	c.mu.Lock()
	c.resolved = &target
	c.mu.Unlock()
	return target, nil
}

func (c *Client) discover(ctx context.Context) (Target, error) {
	if c.override.Service != "" {
		return c.override, nil
	}
	for _, selector := range selectors {
		found, err := c.firstMatching(ctx, selector)
		if err != nil {
			return Target{}, err
		}
		if found.Service != "" {
			return found, nil
		}
	}
	return Target{}, fmt.Errorf("%w: no service matched %s; set --prometheus to namespace/service:port", ErrUnavailable, strings.Join(selectors, " or "))
}

func (c *Client) firstMatching(ctx context.Context, selector string) (Target, error) {
	list, err := c.cs.CoreV1().Services("").List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return Target{}, err
	}
	for i := range list.Items {
		service := &list.Items[i]
		port := webPort(service.Spec.Ports)
		if port == "" {
			continue
		}
		return Target{Namespace: service.Namespace, Service: service.Name, Port: port}, nil
	}
	return Target{}, nil
}

func webPort(ports []corev1.ServicePort) string {
	for _, port := range ports {
		if port.Port == 9090 {
			return "9090"
		}
	}
	for _, port := range ports {
		if strings.Contains(port.Name, "web") || strings.Contains(port.Name, "http") {
			return strconv.Itoa(int(port.Port))
		}
	}
	return ""
}

func (c *Client) probe(ctx context.Context, target Target) (string, error) {
	var lastErr error
	for _, scheme := range []string{"https", "http"} {
		probed := target
		probed.Scheme = scheme
		_, err := c.proxy.Get(ctx, probed, buildPath, nil)
		if err == nil {
			return scheme, nil
		}
		lastErr = err
	}
	return "", fmt.Errorf("%w: %s did not answer over https or http: %w", ErrUnavailable, target, lastErr)
}

func (c *Client) Range(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]api.MetricPoint, error) {
	target, err := c.Target(ctx)
	if err != nil {
		return nil, err
	}
	params := map[string]string{
		"query": query,
		"start": strconv.FormatInt(start.Unix(), 10),
		"end":   strconv.FormatInt(end.Unix(), 10),
		"step":  strconv.Itoa(int(step.Seconds())),
	}
	raw, err := c.proxy.Get(ctx, target, rangePath, params)
	if err != nil {
		return nil, err
	}
	return decodeRange(raw)
}

type rangeResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Data   struct {
		Result []struct {
			Values [][]any `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

func decodeRange(raw []byte) ([]api.MetricPoint, error) {
	var body rangeResponse
	err := json.Unmarshal(raw, &body)
	if err != nil {
		return nil, fmt.Errorf("prometheus returned an unreadable response: %w", err)
	}
	if body.Status != "success" {
		return nil, fmt.Errorf("prometheus rejected the query: %s", body.Error)
	}
	if len(body.Data.Result) == 0 {
		return []api.MetricPoint{}, nil
	}
	values := body.Data.Result[0].Values
	points := make([]api.MetricPoint, 0, len(values))
	for _, pair := range values {
		point, ok := pointOf(pair)
		if !ok {
			continue
		}
		points = append(points, point)
	}
	return points, nil
}

func pointOf(pair []any) (api.MetricPoint, bool) {
	if len(pair) != 2 {
		return api.MetricPoint{}, false
	}
	at, ok := pair[0].(float64)
	if !ok {
		return api.MetricPoint{}, false
	}
	text, ok := pair[1].(string)
	if !ok {
		return api.MetricPoint{}, false
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return api.MetricPoint{}, false
	}
	return api.MetricPoint{At: int64(at), Value: value}, true
}
