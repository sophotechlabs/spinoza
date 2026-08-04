package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/sophotechlabs/spinoza/internal/cluster"
	"github.com/sophotechlabs/spinoza/internal/debugcontainer"
)

var errHelp = errors.New("help requested")

type settings struct {
	addr        string
	openBrowser bool
	tokenFile   string
	logLevel    slog.Level
	showVersion bool
	cluster     cluster.Options
}

func parseFlags(args []string) (settings, error) {
	flags := flag.NewFlagSet("spinoza", flag.ContinueOnError)
	addr := flags.String("addr", envOr("SPINOZA_ADDR", "127.0.0.1:34115"), "listen address; loopback only")
	openBrowser := flags.Bool("open", envBool("SPINOZA_OPEN"), "open the default browser on start")
	tokenFile := flags.String("token-file", envOr("SPINOZA_TOKEN_FILE", ""), "write this run's access token to this file (mode 0600) so scripts can read it")
	logLevel := flags.String("log-level", envOr("SPINOZA_LOG_LEVEL", "info"), "log level: debug, info, warn or error")
	showVersion := flags.Bool("version", false, "print the version and exit")
	debugImage := flags.String("debug-image", envOr("SPINOZA_DEBUG_IMAGE", debugcontainer.DefaultImage), "image used for debug containers")
	kubectlBinary := flags.String("kubectl", envOr("SPINOZA_KUBECTL", debugcontainer.DefaultBinary), "kubectl binary used to create debug containers")
	promSpec := flags.String("prometheus", envOr("SPINOZA_PROMETHEUS", ""), "prometheus service as namespace/service:port; discovered when empty")
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
	return settings{
		addr:        *addr,
		openBrowser: *openBrowser,
		tokenFile:   *tokenFile,
		logLevel:    level,
		showVersion: *showVersion,
		cluster: cluster.Options{
			DebugImage:    *debugImage,
			KubectlBinary: *kubectlBinary,
			PromSpec:      *promSpec,
		},
	}, nil
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
	err := os.WriteFile(path, []byte(token+"\n"), 0o600)
	if err != nil {
		return fmt.Errorf("token file: %w", err)
	}
	return nil
}
