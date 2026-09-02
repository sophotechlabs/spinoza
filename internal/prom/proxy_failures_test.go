package prom

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

type proxyRoundTripper func(*http.Request) (*http.Response, error)

func (roundTrip proxyRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

type trackedProxyBody struct {
	reader io.Reader
	closed bool
}

func (body *trackedProxyBody) Read(buffer []byte) (int, error) {
	return body.reader.Read(buffer)
}

func (body *trackedProxyBody) Close() error {
	body.closed = true
	return nil
}

func proxyClientset(t *testing.T, roundTrip http.RoundTripper) kubernetes.Interface {
	t.Helper()
	clientset, err := kubernetes.NewForConfigAndClient(
		&rest.Config{Host: "https://apiserver.invalid"},
		&http.Client{Transport: roundTrip},
	)
	if err != nil {
		t.Fatalf("clientset: %v", err)
	}
	return clientset
}

func proxyTarget() Target {
	return Target{Namespace: "monitoring", Service: "prometheus", Port: "9090", Scheme: "https"}
}

func TestServiceProxyRefusesAClientWithoutARESTTransport(t *testing.T) {
	proxy := &serviceProxy{cs: k8sfake.NewClientset()}

	_, err := proxy.Get(t.Context(), proxyTarget(), rangePath, nil)

	if err == nil {
		t.Fatal("a client with no REST transport was allowed to proxy prometheus")
	}
	if !strings.Contains(err.Error(), "cannot safely proxy") {
		t.Fatalf("error = %q, want the unavailable transport named", err)
	}
}

func TestServiceProxySurfacesATransportFailure(t *testing.T) {
	failure := errors.New("apiserver connection reset")
	clientset := proxyClientset(t, proxyRoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Accept") != "application/json" {
			t.Errorf("accept = %q, want application/json", request.Header.Get("Accept"))
		}
		if _, bounded := request.Context().Deadline(); !bounded {
			t.Error("prometheus proxy request has no deadline")
		}
		return nil, failure
	}))
	proxy := &serviceProxy{cs: clientset}

	_, err := proxy.Get(t.Context(), proxyTarget(), rangePath, nil)

	if !errors.Is(err, failure) {
		t.Fatalf("error = %v, want the transport failure", err)
	}
}

func TestServiceProxyFallsBackToTheDefaultHTTPClient(t *testing.T) {
	apiserver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("healthy"))
	}))
	t.Cleanup(apiserver.Close)
	clientset, err := kubernetes.NewForConfig(&rest.Config{Host: apiserver.URL})
	if err != nil {
		t.Fatalf("clientset: %v", err)
	}
	restClient, ok := clientset.CoreV1().RESTClient().(*rest.RESTClient)
	if !ok {
		t.Fatalf("REST client = %T", clientset.CoreV1().RESTClient())
	}
	restClient.Client = nil
	proxy := &serviceProxy{cs: clientset}

	body, err := proxy.Get(t.Context(), proxyTarget(), rangePath, nil)
	if err != nil {
		t.Fatalf("proxy with the default HTTP client: %v", err)
	}
	if string(body) != "healthy" {
		t.Fatalf("body = %q, want the proxied response", body)
	}
}

func TestServiceProxyClosesAResponseThatFailsWhileReading(t *testing.T) {
	failure := errors.New("response stream reset")
	body := &trackedProxyBody{reader: io.MultiReader(strings.NewReader("partial"), errorReader{err: failure})}
	clientset := proxyClientset(t, proxyRoundTripper(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       body,
			Request:    request,
		}, nil
	}))
	proxy := &serviceProxy{cs: clientset}

	_, err := proxy.Get(t.Context(), proxyTarget(), rangePath, nil)

	if !errors.Is(err, failure) {
		t.Fatalf("error = %v, want the response read failure", err)
	}
	if !body.closed {
		t.Fatal("the failed response body was left open")
	}
}

func TestServiceProxyPreservesAnAPIServerRejection(t *testing.T) {
	body := &trackedProxyBody{reader: strings.NewReader("services/proxy is forbidden")}
	clientset := proxyClientset(t, proxyRoundTripper(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Body:       body,
			Request:    request,
		}, nil
	}))
	proxy := &serviceProxy{cs: clientset}

	_, err := proxy.Get(t.Context(), proxyTarget(), rangePath, nil)

	if !apierrors.IsForbidden(err) {
		t.Fatalf("error = %v, want a Kubernetes forbidden error", err)
	}
	if !strings.Contains(err.Error(), "services/proxy is forbidden") {
		t.Fatalf("error = %q, want the apiserver response", err)
	}
	if !body.closed {
		t.Fatal("the rejected response body was left open")
	}
}

func TestServiceProxyAcceptsAResponseAtTheSizeLimit(t *testing.T) {
	body := &trackedProxyBody{reader: io.LimitReader(zeroReader{}, maxResponseBytes)}
	clientset := proxyClientset(t, proxyRoundTripper(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       body,
			Request:    request,
		}, nil
	}))
	proxy := &serviceProxy{cs: clientset}

	response, err := proxy.Get(t.Context(), proxyTarget(), rangePath, nil)
	if err != nil {
		t.Fatalf("response at the size limit: %v", err)
	}
	if len(response) != maxResponseBytes {
		t.Fatalf("response bytes = %d, want %d", len(response), maxResponseBytes)
	}
	if !body.closed {
		t.Fatal("the accepted response body was left open")
	}
}

type errorReader struct {
	err error
}

func (reader errorReader) Read([]byte) (int, error) {
	return 0, reader.err
}

type zeroReader struct{}

func (zeroReader) Read(buffer []byte) (int, error) {
	clear(buffer)
	return len(buffer), nil
}
