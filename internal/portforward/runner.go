package portforward

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	streamhttp "k8s.io/streaming/pkg/httpstream"
)

type dialerFactory func(namespace, pod string) (streamhttp.Dialer, error)

type streamRunner struct {
	dialerFor dialerFactory
}

func NewRunner(cs kubernetes.Interface, config *restclient.Config) Runner {
	return &streamRunner{
		dialerFor: func(namespace, pod string) (streamhttp.Dialer, error) {
			return apiDialer(cs, config, namespace, pod)
		},
	}
}

func apiDialer(cs kubernetes.Interface, config *restclient.Config, namespace, pod string) (streamhttp.Dialer, error) {
	endpoint := cs.CoreV1().RESTClient().
		Post().
		Resource("pods").
		Namespace(namespace).
		Name(pod).
		SubResource("portforward").
		URL()
	return fallbackDialer(config, endpoint)
}

func fallbackDialer(config *restclient.Config, endpoint *url.URL) (streamhttp.Dialer, error) {
	transport, upgrader, err := spdy.RoundTripperFor(config)
	if err != nil {
		return nil, err
	}
	overSPDY := spdy.NewDialerForStreaming(upgrader, &http.Client{Transport: transport}, http.MethodPost, endpoint)
	overWebsocket, err := portforward.NewSPDYOverWebsocketDialerForStreaming(endpoint, config)
	if err != nil {
		return nil, err
	}
	return portforward.NewFallbackDialerForStreaming(overWebsocket, overSPDY, shouldFallback), nil
}

func shouldFallback(err error) bool {
	if streamhttp.IsUpgradeFailure(err) {
		return true
	}
	return streamhttp.IsHTTPSProxyError(err)
}

func (s *streamRunner) Run(ctx context.Context, namespace, pod string, remotePort int32, ready chan<- int32, stop <-chan struct{}) error {
	dialer, err := s.dialerFor(namespace, pod)
	if err != nil {
		return err
	}

	forwarderReady := make(chan struct{})
	forwarder, err := portforward.NewForStreaming(
		dialer,
		[]string{fmt.Sprintf("0:%d", remotePort)},
		stop,
		forwarderReady,
		io.Discard,
		io.Discard,
	)
	if err != nil {
		return err
	}

	go announce(ctx, forwarder, forwarderReady, ready)
	return forwarder.ForwardPorts()
}

func announce(ctx context.Context, forwarder *portforward.PortForwarder, forwarderReady <-chan struct{}, ready chan<- int32) {
	select {
	case <-ctx.Done():
		return
	case <-forwarderReady:
	}
	ports, err := forwarder.GetPorts()
	if err != nil {
		return
	}
	if len(ports) == 0 {
		return
	}
	select {
	case ready <- int32(ports[0].Local):
	default:
	}
}
