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

	"github.com/sophotechlabs/spinoza/internal/auth"
	"github.com/sophotechlabs/spinoza/internal/safe"
)

type dialerFactory func(ctx context.Context, namespace, pod string) (streamhttp.Dialer, error)

type streamRunner struct {
	dialerFor dialerFactory
}

func NewRunner(cs kubernetes.Interface, config *restclient.Config) Runner {
	return &streamRunner{
		dialerFor: func(ctx context.Context, namespace, pod string) (streamhttp.Dialer, error) {
			return apiDialer(ctx, cs, config, namespace, pod)
		},
	}
}

func apiDialer(
	ctx context.Context,
	cs kubernetes.Interface,
	config *restclient.Config,
	namespace, pod string,
) (streamhttp.Dialer, error) {
	endpoint := cs.CoreV1().RESTClient().
		Post().
		Resource("pods").
		Namespace(namespace).
		Name(pod).
		SubResource("portforward").
		URL()
	return fallbackDialer(actingAs(ctx, config), endpoint)
}

func actingAs(ctx context.Context, config *restclient.Config) *restclient.Config {
	who, ok := auth.ActingAs(ctx)
	if !ok {
		return config
	}
	copied := restclient.CopyConfig(config)
	copied.Impersonate = restclient.ImpersonationConfig{UserName: who.User, Groups: who.Groups}
	return copied
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

func (s *streamRunner) Run(ctx context.Context, namespace, pod string, localPort, remotePort int32, ready chan<- int32, stop <-chan struct{}) error {
	dialer, err := s.dialerFor(ctx, namespace, pod)
	if err != nil {
		return err
	}

	done := make(chan struct{})
	defer close(done)
	halt := make(chan struct{})
	safe.Go("relaying the stop signal", func() { relayStop(ctx, stop, done, halt) })

	forwarderReady := make(chan struct{})
	forwarder, err := portforward.NewForStreaming(
		dialer,
		[]string{fmt.Sprintf("%d:%d", localPort, remotePort)},
		halt,
		forwarderReady,
		io.Discard,
		io.Discard,
	)
	if err != nil {
		return err
	}

	safe.Go("announcing the local port", func() { announce(ctx, forwarder, forwarderReady, done, ready) })
	return forwarder.ForwardPorts()
}

func relayStop(ctx context.Context, stop, done <-chan struct{}, halt chan<- struct{}) {
	select {
	case <-ctx.Done():
		close(halt)
	case <-stop:
		close(halt)
	case <-done:
	}
}

func announce(
	ctx context.Context,
	forwarder *portforward.PortForwarder,
	forwarderReady <-chan struct{},
	done <-chan struct{},
	ready chan<- int32,
) {
	select {
	case <-ctx.Done():
		return
	case <-done:
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
