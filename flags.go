package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/sophotechlabs/spinoza/internal/cluster"
	"github.com/sophotechlabs/spinoza/internal/debugcontainer"
)

var errHelp = errors.New("help requested")

type settings struct {
	addr        string
	openBrowser bool
	tokenFile   string
	cluster     cluster.Options
}

func parseFlags(args []string) (settings, error) {
	flags := flag.NewFlagSet("spinoza", flag.ContinueOnError)
	addr := flags.String("addr", "127.0.0.1:34115", "listen address; loopback only")
	openBrowser := flags.Bool("open", false, "open the default browser on start")
	tokenFile := flags.String("token-file", "", "write this run's access token to this file (mode 0600) so scripts can read it")
	debugImage := flags.String("debug-image", debugcontainer.DefaultImage, "image used for debug containers")
	kubectlBinary := flags.String("kubectl", debugcontainer.DefaultBinary, "kubectl binary used to create debug containers")
	promSpec := flags.String("prometheus", "", "prometheus service as namespace/service:port; discovered when empty")
	err := flags.Parse(args)
	if errors.Is(err, flag.ErrHelp) {
		return settings{}, errHelp
	}
	if err != nil {
		return settings{}, err
	}
	return settings{
		addr:        *addr,
		openBrowser: *openBrowser,
		tokenFile:   *tokenFile,
		cluster: cluster.Options{
			DebugImage:    *debugImage,
			KubectlBinary: *kubectlBinary,
			PromSpec:      *promSpec,
		},
	}, nil
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
