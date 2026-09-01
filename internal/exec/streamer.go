package exec

import (
	"context"
	"net/http"
	"net/url"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	streamhttp "k8s.io/streaming/pkg/httpstream"
)

type apiStreamer struct {
	cs     kubernetes.Interface
	config *restclient.Config
}

func NewStreamer(cs kubernetes.Interface, config *restclient.Config) Streamer {
	return &apiStreamer{cs: cs, config: config}
}

func (a *apiStreamer) Stream(ctx context.Context, req Request, opts Options) error {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	endpoint := a.cs.CoreV1().RESTClient().
		Post().
		Resource("pods").
		Namespace(req.Namespace).
		Name(req.Pod).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: req.Container,
			Command:   opts.Command,
			Stdin:     true,
			Stdout:    true,
			Stderr:    false,
			TTY:       true,
		}, scheme.ParameterCodec).
		URL()

	executor, err := fallbackExecutor(a.config, endpoint)
	if err != nil {
		return err
	}
	return executor.StreamWithContext(streamCtx, remotecommand.StreamOptions{
		Stdin:             opts.Stdin,
		Stdout:            opts.Stdout,
		Tty:               true,
		TerminalSizeQueue: &sizeQueue{ctx: streamCtx, resize: opts.Resize},
	})
}

func fallbackExecutor(config *restclient.Config, endpoint *url.URL) (remotecommand.Executor, error) {
	overWebsocket, err := remotecommand.NewWebSocketExecutor(config, http.MethodGet, endpoint.String())
	if err != nil {
		return nil, err
	}
	overSPDY, err := remotecommand.NewSPDYExecutor(config, http.MethodPost, endpoint)
	if err != nil {
		return nil, err
	}
	return remotecommand.NewFallbackExecutor(overWebsocket, overSPDY, shouldFallback)
}

func shouldFallback(err error) bool {
	if streamhttp.IsUpgradeFailure(err) {
		return true
	}
	return streamhttp.IsHTTPSProxyError(err)
}

type sizeQueue struct {
	ctx    context.Context
	resize <-chan Size
}

func (q *sizeQueue) Next() *remotecommand.TerminalSize {
	select {
	case <-q.ctx.Done():
		return nil
	case size, ok := <-q.resize:
		if !ok {
			return nil
		}
		return &remotecommand.TerminalSize{Width: size.Cols, Height: size.Rows}
	}
}
