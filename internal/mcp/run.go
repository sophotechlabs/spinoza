package mcp

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/kube"
	"github.com/sophotechlabs/spinoza/internal/prom"
)

var ErrHelp = errors.New("help was asked for")

type Settings struct {
	Kubeconfig string
	Context    string
	PromSpec   string
	AllowWrite bool
	LogLines   int
	SyncWait   time.Duration
	Args       []string
}

func Parse(argv []string, out io.Writer) (Settings, error) {
	flags := flag.NewFlagSet("spinoza-mcp", flag.ContinueOnError)
	flags.SetOutput(out)
	kubeconfig := flags.String("kubeconfig", "", "kubeconfig to read; the usual lookup rules when empty")
	kubeContext := flags.String("context", "", "context to use; the kubeconfig's current one when empty")
	promSpec := flags.String("prometheus", "", "namespace/service:port of Prometheus; discovered when empty")
	allowWrite := flags.Bool("allow-write", false, "offer the five tools that change the cluster")
	logLines := flags.Int("log-lines", defaultLogLines, "most log lines any one tool returns")
	syncWait := flags.Duration("sync-timeout", 30*time.Second, "how long to wait for an informer cache")
	flags.Usage = func() {
		fmt.Fprintln(out, "spinoza-mcp reads one Kubernetes cluster for an MCP client.")
		fmt.Fprintln(out, "  spinoza-mcp                    serve MCP on stdin and stdout")
		fmt.Fprintln(out, "  spinoza-mcp tools              list the tools and what each one does")
		fmt.Fprintln(out, "  spinoza-mcp call NAME k=v ...  run one tool and print its JSON")
		flags.PrintDefaults()
	}
	if err := flags.Parse(argv); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return Settings{}, ErrHelp
		}
		return Settings{}, err
	}
	return Settings{
		Kubeconfig: *kubeconfig,
		Context:    *kubeContext,
		PromSpec:   *promSpec,
		AllowWrite: *allowWrite,
		LogLines:   *logLines,
		SyncWait:   *syncWait,
		Args:       flags.Args(),
	}, nil
}

func (s *Server) Dispatch(ctx context.Context, opts Settings, in io.Reader, out io.Writer) error {
	if len(opts.Args) == 0 {
		return s.Serve(ctx, in, out)
	}
	switch opts.Args[0] {
	case "tools":
		return s.List(out)
	case "call":
		if len(opts.Args) < 2 {
			return errors.New("call needs a tool name")
		}
		return s.Call(ctx, out, opts.Args[1], opts.Args[2:])
	default:
		return fmt.Errorf("unknown command %q; use tools or call", opts.Args[0])
	}
}

func PromFor(ref api.ContextRef, opts Settings) Prometheus {
	target, err := prom.ParseTarget(opts.PromSpec)
	if err != nil {
		return nil
	}
	bundle, err := kube.LoadContext(ref, kube.Options{Kubeconfig: opts.Kubeconfig})
	if err != nil {
		return nil
	}
	return prom.NewClient(bundle.Clientset, target)
}
