package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/sophotechlabs/spinoza/internal/cluster"
	"github.com/sophotechlabs/spinoza/internal/debugcontainer"
	"github.com/sophotechlabs/spinoza/internal/helm"
	"github.com/sophotechlabs/spinoza/internal/imagepin"
	"github.com/sophotechlabs/spinoza/internal/nodeshell"
)

var errHelp = errors.New("help requested")

var errUnknownLogFormat = errors.New("log-format has to be json or text")

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
	addr           string
	openBrowser    bool
	tokenFile      string
	logLevel       slog.Level
	logFormat      string
	metricsAddr    string
	recordSessions bool
	auditInterval  time.Duration
	auditWebhook   string
	auditRetention time.Duration
	showVersion    bool
	showLicense    bool
	pprof          bool
	nodeShell      bool
	startView      string
	serve          serving
	cluster        cluster.Options
}

func parseFlags(args []string) (settings, error) {
	flags := flag.NewFlagSet("spinoza", flag.ContinueOnError)
	addr := flags.String("addr", envOr("SPINOZA_ADDR", "127.0.0.1:34115"), "listen address; loopback only")
	openBrowser := flags.Bool("open", envBool("SPINOZA_OPEN"), "open the default browser on start")
	tokenFile := flags.String("token-file", envOr("SPINOZA_TOKEN_FILE", ""), "write this run's access token to this file (mode 0600) so scripts can read it")
	logLevel := flags.String("log-level", envOr("SPINOZA_LOG_LEVEL", "info"), "log level: debug, info, warn or error")
	logFormat := flags.String("log-format", envOr("SPINOZA_LOG_FORMAT", ""), "log format: json or text; text on your own machine and json when serving a cluster")
	metricsAddr := flags.String("metrics-addr", envOr("SPINOZA_METRICS_ADDR", ""), "extra listen address serving only /metrics, with no session; keep it inside the cluster")
	recordSessions := flags.Bool("record-sessions", envBool("SPINOZA_RECORD_SESSIONS"), "keep a transcript of every exec and node shell beside its audit entry; admins read them")
	auditInterval := flags.Duration("audit-interval", envDuration("SPINOZA_AUDIT_INTERVAL", 0), "re-run the checks on this interval and keep what each run found; off when zero")
	auditWebhook := flags.String("audit-webhook", envOr("SPINOZA_AUDIT_WEBHOOK", ""), "post a json summary here when a scheduled run differs from the baseline")
	auditRetention := flags.Duration("audit-retention", envDuration("SPINOZA_AUDIT_RETENTION", 0), "how long recorded changes and audit runs are kept; kept for good when zero")
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
	format, formatErr := parseLogFormat(*logFormat)
	if formatErr != nil {
		return settings{}, formatErr
	}
	served, servedErr := serving.settings()
	if servedErr != nil {
		return settings{}, servedErr
	}
	clusterSettings := cluster.Options{
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
	}
	if !*showVersion && !*showLicense {
		runnable := runnableSettings{
			served:    served,
			cluster:   clusterSettings,
			nodeShell: *nodeShell,
			addr:      *addr,
			metrics:   *metricsAddr,
			webhook:   *auditWebhook,
			interval:  *auditInterval,
		}
		checkErr := runnable.check()
		if checkErr != nil {
			return settings{}, checkErr
		}
	}
	return settings{
		addr:           listenAddress(*addr, served.on, wasGiven(flags, "addr")),
		openBrowser:    *openBrowser,
		tokenFile:      *tokenFile,
		logLevel:       level,
		logFormat:      logFormatFor(format, served.on),
		metricsAddr:    *metricsAddr,
		recordSessions: *recordSessions,
		auditInterval:  *auditInterval,
		auditWebhook:   *auditWebhook,
		auditRetention: *auditRetention,
		showVersion:    *showVersion,
		showLicense:    *showLicense,
		pprof:          *profiler,
		nodeShell:      *nodeShell,
		startView:      *startView,
		serve:          served,
		cluster:        clusterSettings,
	}, nil
}

const minAuditInterval = time.Minute

type runnableSettings struct {
	served    serving
	cluster   cluster.Options
	nodeShell bool
	addr      string
	metrics   string
	webhook   string
	interval  time.Duration
}

func (rs runnableSettings) check() error {
	servedErr := rs.served.check()
	if servedErr != nil {
		return servedErr
	}
	limitErr := validateClusterLimits(rs.cluster)
	if limitErr != nil {
		return limitErr
	}
	if rs.nodeShell && !imagepin.Valid(rs.cluster.NodeShellImage) {
		return errors.New("node-shell-image must be pinned by sha256 digest")
	}
	metricsErr := checkMetricsAddr(rs.metrics, rs.addr)
	if metricsErr != nil {
		return metricsErr
	}
	webhookErr := checkAuditWebhook(rs.webhook)
	if webhookErr != nil {
		return webhookErr
	}
	if rs.interval != 0 && rs.interval < minAuditInterval {
		return fmt.Errorf("audit-interval has to be at least %s; the checks read the whole cluster", minAuditInterval)
	}
	return nil
}

func checkMetricsAddr(metrics, main string) error {
	if metrics == "" {
		return nil
	}
	_, _, err := net.SplitHostPort(metrics)
	if err != nil {
		return fmt.Errorf("metrics-addr %q must be host:port", metrics)
	}
	if metrics == main {
		return errors.New("metrics-addr has to differ from addr; the metrics port answers without a session")
	}
	return nil
}

func checkAuditWebhook(raw string) error {
	if raw == "" {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("audit-webhook %q is not a url", raw)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("audit-webhook %q has to be http or https", raw)
	}
	if parsed.Host == "" {
		return fmt.Errorf("audit-webhook %q names no host", raw)
	}
	return nil
}

func validateClusterLimits(options cluster.Options) error {
	qps := float64(options.ClientQPS)
	if qps <= 0 || math.IsNaN(qps) || math.IsInf(qps, 0) {
		return errors.New("qps must be a finite positive number")
	}
	if options.ClientBurst <= 0 {
		return errors.New("burst must be positive")
	}
	if options.SyncTimeout <= 0 {
		return errors.New("sync-timeout must be positive")
	}
	if options.WarmConcurrency <= 0 {
		return errors.New("warm-concurrency must be positive")
	}
	if options.CountBudget <= 0 {
		return errors.New("count-budget must be positive")
	}
	if options.CountPerType <= 0 {
		return errors.New("count-timeout must be positive")
	}
	if options.CountConcurrency <= 0 {
		return errors.New("count-concurrency must be positive")
	}
	return nil
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
