package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/sophotechlabs/spinoza/internal/cluster"
	"github.com/sophotechlabs/spinoza/internal/debugcontainer"
	"github.com/sophotechlabs/spinoza/internal/helm"
	"github.com/sophotechlabs/spinoza/internal/nodeshell"
)

var errHelp = errors.New("help requested")

const (
	defaultQPS              = 50
	defaultBurst            = 100
	defaultSync             = 30 * time.Second
	defaultWarm             = 8
	defaultCountBudget      = 20 * time.Second
	defaultCountPerType     = 5 * time.Second
	defaultCountConcurrency = 24
)

type settings struct {
	addr        string
	openBrowser bool
	tokenFile   string
	logLevel    slog.Level
	showVersion bool
	showLicense bool
	pprof       bool
	nodeShell   bool
	startView   string
	serve       serving
	cluster     cluster.Options
}

func parseFlags(args []string) (settings, error) {
	flags := flag.NewFlagSet("spinoza", flag.ContinueOnError)
	addr := flags.String("addr", envOr("SPINOZA_ADDR", "127.0.0.1:34115"), "listen address; loopback only")
	openBrowser := flags.Bool("open", envBool("SPINOZA_OPEN"), "open the default browser on start")
	tokenFile := flags.String("token-file", envOr("SPINOZA_TOKEN_FILE", ""), "write this run's access token to this file (mode 0600) so scripts can read it")
	logLevel := flags.String("log-level", envOr("SPINOZA_LOG_LEVEL", "info"), "log level: debug, info, warn or error")
	showVersion := flags.Bool("version", false, "print the version and exit")
	showLicense := flags.Bool("license", false, "print the license and exit")
	profiler := flags.Bool("pprof", envBool("SPINOZA_PPROF"), "mount net/http/pprof under /debug/pprof, behind the same auth; off by default")
	debugImage := flags.String("debug-image", envOr("SPINOZA_DEBUG_IMAGE", debugcontainer.DefaultImage), "image used for debug containers")
	nodeShell := flags.Bool("node-shell", envBool("SPINOZA_NODE_SHELL"), "allow a root shell on a node, which creates a privileged pod")
	nodeShellImage := flags.String("node-shell-image", envOr("SPINOZA_NODE_SHELL_IMAGE", debugcontainer.DefaultImage), "image the node shell pod runs")
	nodeShellNamespace := flags.String("node-shell-namespace", envOr("SPINOZA_NODE_SHELL_NAMESPACE", nodeshell.DefaultNamespace), "namespace the node shell pod is created in")
	kubectlBinary := flags.String("kubectl", envOr("SPINOZA_KUBECTL", debugcontainer.DefaultBinary), "kubectl binary used to create debug containers")
	helmBinary := flags.String("helm", envOr("SPINOZA_HELM", helm.DefaultBinary), "helm binary used to roll back and uninstall releases")
	promSpec := flags.String("prometheus", envOr("SPINOZA_PROMETHEUS", ""), "prometheus service as namespace/service:port; discovered when empty")
	kubeconfig := flags.String("kubeconfig", envOr("SPINOZA_KUBECONFIG", ""), "kubeconfig to read; the usual lookup rules when empty")
	startView := flags.String("view", envOr("SPINOZA_START_VIEW", ""), "view to open on when nothing else asks for one")
	startContext := flags.String("context", envOr("SPINOZA_START_CONTEXT", ""), "kubeconfig context to open on start; the current one when empty")
	clientQPS := flags.Float64("qps", envFloat("SPINOZA_QPS", defaultQPS), "apiserver requests per second this client allows itself")
	clientBurst := flags.Int("burst", envInt("SPINOZA_BURST", defaultBurst), "apiserver requests this client may burst to")
	syncTimeout := flags.Duration("sync-timeout", envDuration("SPINOZA_SYNC_TIMEOUT", defaultSync), "how long one resource type may take to fill its cache")
	warmConcurrency := flags.Int("warm-concurrency", envInt("SPINOZA_WARM_CONCURRENCY", defaultWarm), "resource types warmed at once for the gitops and flux views")
	countBudget := flags.Duration("count-budget", envDuration("SPINOZA_COUNT_BUDGET", defaultCountBudget), "total time the sidebar counts may take")
	countPerType := flags.Duration("count-timeout", envDuration("SPINOZA_COUNT_TIMEOUT", defaultCountPerType), "time one resource type may take to be counted")
	countConcurrency := flags.Int("count-concurrency", envInt("SPINOZA_COUNT_CONCURRENCY", defaultCountConcurrency), "resource types counted at once")
	serving := registerCluster(flags)
	err := flags.Parse(args)
	if errors.Is(err, flag.ErrHelp) {
		return settings{}, errHelp
	}
	if err != nil {
		return settings{}, err
	}
	level, levelErr := parseLevel(*logLevel)
	if levelErr != nil {
		return settings{}, levelErr
	}
	served, servedErr := serving.settings()
	if servedErr != nil {
		return settings{}, servedErr
	}
	if !*showVersion && !*showLicense {
		checkErr := served.check()
		if checkErr != nil {
			return settings{}, checkErr
		}
	}
	return settings{
		addr:        listenAddress(*addr, served.on, wasGiven(flags, "addr")),
		openBrowser: *openBrowser,
		tokenFile:   *tokenFile,
		logLevel:    level,
		showVersion: *showVersion,
		showLicense: *showLicense,
		pprof:       *profiler,
		nodeShell:   *nodeShell,
		startView:   *startView,
		serve:       served,
		cluster: cluster.Options{
			Impersonate:      served.on && served.impersonate,
			DebugImage:       *debugImage,
			NodeShellImage:   *nodeShellImage,
			NodeShellNS:      *nodeShellNamespace,
			KubectlBinary:    *kubectlBinary,
			HelmBinary:       *helmBinary,
			PromSpec:         *promSpec,
			Kubeconfig:       *kubeconfig,
			Context:          *startContext,
			ClientQPS:        float32(*clientQPS),
			ClientBurst:      *clientBurst,
			SyncTimeout:      *syncTimeout,
			WarmConcurrency:  *warmConcurrency,
			CountBudget:      *countBudget,
			CountPerType:     *countPerType,
			CountConcurrency: *countConcurrency,
		},
	}, nil
}

func wasGiven(flags *flag.FlagSet, name string) bool {
	found := false
	flags.Visit(func(one *flag.Flag) {
		if one.Name == name {
			found = true
		}
	})
	return found
}

func listenAddress(addr string, serving, given bool) string {
	if !serving || given {
		return addr
	}
	_, fromEnv := os.LookupEnv("SPINOZA_ADDR")
	if fromEnv {
		return addr
	}
	return clusterAddr
}

func parseLevel(name string) (slog.Level, error) {
	switch name {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("log level %q is not one of debug, info, warn, error", name)
	}
}

func envOr(name, fallback string) string {
	value, present := os.LookupEnv(name)
	if !present {
		return fallback
	}
	return value
}

func envInt(name string, fallback int) int {
	value, present := os.LookupEnv(name)
	if !present {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envFloat(name string, fallback float64) float64 {
	value, present := os.LookupEnv(name)
	if !present {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value, present := os.LookupEnv(name)
	if !present {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(name string) bool {
	value, present := os.LookupEnv(name)
	if !present {
		return false
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false
	}
	return parsed
}

func settingsFromArgs() (settings, error) {
	return parseFlags(os.Args[1:])
}

func writeTokenFile(path, token string) error {
	if path == "" {
		return nil
	}
	_ = os.Remove(path)
	err := os.WriteFile(path, []byte(token+"\n"), 0o600)
	if err != nil {
		return fmt.Errorf("token file: %w", err)
	}
	return nil
}
