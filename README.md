# Spinoza

[![go](https://img.shields.io/github/actions/workflow/status/sophotechlabs/spinoza/go.yaml?branch=main&label=go)](https://github.com/sophotechlabs/spinoza/actions/workflows/go.yaml)
[![frontend](https://img.shields.io/github/actions/workflow/status/sophotechlabs/spinoza/frontend.yaml?branch=main&label=frontend)](https://github.com/sophotechlabs/spinoza/actions/workflows/frontend.yaml)
[![repo](https://img.shields.io/github/actions/workflow/status/sophotechlabs/spinoza/repo.yaml?branch=main&label=repo)](https://github.com/sophotechlabs/spinoza/actions/workflows/repo.yaml)
[![go coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/sophotechlabs/spinoza/badges/coverage-go.json)](https://github.com/sophotechlabs/spinoza/actions/workflows/go.yaml)
[![web coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/sophotechlabs/spinoza/badges/coverage-web.json)](https://github.com/sophotechlabs/spinoza/actions/workflows/frontend.yaml)
[![license](https://img.shields.io/badge/license-FSL--1.1--ALv2-blue)](LICENSE)

A self-hosted Kubernetes GUI: Go backend with client-go informers, React frontend, one binary. Runs as a browser tab or a desktop window. [spinoza.tech](https://spinoza.tech)

## Install

```sh
curl -fsSL https://spinoza.tech/install.sh | sh
```

The binary goes to `~/.local/bin`, or `/usr/local/bin` as root; `SPINOZA_INSTALL_DIR` overrides both. Checksums are verified before anything is written. On macOS the desktop app lands in `/Applications`.

```sh
spinoza --open          # browser
open -a Spinoza         # desktop window, macOS
```

Archives are on the [releases page](https://github.com/sophotechlabs/spinoza/releases): tarballs for Linux and macOS, zips for Windows. `SPINOZA_VERSION=v1.8.1` pins the installer.

Needs a kubeconfig. Upgrades, rollbacks and debug containers call `helm` and `kubectl`.

## Every GVR discovery reports

One informer-backed view per type, so CRDs need no per-type code. Snapshot, then deltas over a WebSocket keyed by uid. Field-aware filter completed from what the cluster reported, namespace scope, failing-object badges, bulk restart and delete.

![Spinoza pods table with the inspect drawer open on coredns: ports, metadata, conditions, containers and owner references](docs/images/pods.png)

## GitOps

Flux and Argo CD side by side: cluster sync, controller health, per-kind lists, status tiles. Reconcile, suspend and resume Flux objects. Sync and refresh Argo Applications.

![Spinoza Flux overview: all systems operational, cluster sync from a GitRepository, controller health, and counts for appliers, sources and image automation](docs/images/flux-overview.png)

The dependency graph is laid out by what manages what. Argo's is the app-of-apps, ApplicationSets above what they generate. A node opens in the drawer with its actions.

![Spinoza GitOps dependency graph: sources, Kustomizations and HelmReleases with the edges that manage and depend on each other](docs/images/gitops-graph.png)

## Helm without the binary

Releases read from Helm's own storage, either driver. Chart and app version, what your repos offer, revision, status, values, notes, rendered manifest, history. OCI end to end. Upgrade behind a server-rendered manifest diff; rollback and uninstall from the same panel. A Flux-owned release links to its HelmRelease instead of offering an upgrade button.

![Spinoza Helm releases with chart and app versions, revision and status, and the selected release docked below showing the resources it rendered](docs/images/helm-releases.png)

Selecting a release docks its detail as a panel. Drag it to any side, resize it, collapse it; the placement persists, and `release` and `releaseNs` in the URL carry it through a reload or a back button. Clicking a rendered resource opens that object in its own table with the row selected and the drawer open, while the release stays docked underneath.

![Spinoza deployments table scoped to the cert-manager namespace with the deployment selected, its inspect drawer open on metadata and conditions, and the Helm release still docked below on its resource list](docs/images/helm-resources.png)

## Inspect and edit

Metadata, conditions, events and live YAML in Monaco. Schema-aware completion from the cluster's own OpenAPI, server-side apply and delete. An edited draft survives a background reload.

![Spinoza deployments table with the inspect drawer open on live YAML in Monaco, with apply, revert and delete](docs/images/inspect-yaml.png)

## Port-forwarding

Any pod or service on localhost, no kubectl. Forwards survive navigation, listed and stopped from one panel.

![Spinoza pods table with the inspect drawer open on Grafana's ports: 3000 forwarded to a local port with open and stop controls, and a toast confirming the forward](docs/images/port-forward.png)

## Write actions

Scale, rollout restart, cordon, uncordon, drain. Drain shows its eviction plan before it runs: what it evicts, what it leaves, what it blocks and why. Protected clusters require the object name typed out.

![Spinoza drain plan for a node: how many pods it would evict, leave in place and block, the reason for each, and the drain button disabled behind a checkbox](docs/images/drain-plan.png)

## Also

- **Cluster overview**: server version, node readiness, allocatable against live usage, pods by phase, recent warning events.
- **Exec** over `v5.channel.k8s.io` into xterm. Shell-less images get an ephemeral debug container, on a verdict cached per image digest.
- **Logs** per container. Pausing follow stops the scroll, not the stream.
- **Metrics**: metrics-server in the tables, CPU and memory history from Prometheus through the apiserver proxy. `--prometheus namespace/service:port` overrides discovery. With no Prometheus to ask, spinoza samples metrics-server itself while the window is open and says so on the chart.
- **Update check**: asks GitHub once per run whether a newer release exists, and offers the install command if so. It never installs anything. `--update-check=false` turns it off, `--update-endpoint URL` points it elsewhere.
- **Kubeconfigs** added by path, referenced in place, never copied or merged. Contexts grouped per file, listed in `kubeconfigs.json`. `--kubeconfig PATH` replaces the default lookup for one run.
- **Nine themes**, contrast-gated in CI, plus your own as JSON. Screenshots here are Borg.

## Security

Binds loopback only; rejects non-local Host or Origin. Every route and both WebSockets require the token minted for that run, as the `X-Spinoza-Token` header, a `token` query parameter, or the cookie the page keeps. `--token-file PATH` writes it at mode 0600. Exits when the last view closes.

Outbound: your apiserver, the chart repos you configured, and one request per run to the GitHub releases API for the update check — `--update-check=false` stops that one.

## Develop

Toolchain pinned in `mise.toml` (Go 1.26, Node 24); `mise install` gets them.

```sh
just deps      # frontend dependencies, once
just run       # build the frontend and the binary, then start
just dev-api   # Go server on :34115
just dev-web   # Vite, proxying to the API
just rund      # build and open the desktop app
```

Vite serves its own `index.html` without the token, so open `http://localhost:5173/?token=<the token dev-api printed>`.

The binary embeds the built frontend; `just stub-assets` writes a placeholder for the Go-only recipes.

`just check` before pushing. `just ci` runs everything CI does: cross-compilation, the coverage gate, `govulncheck`, dead-code and unused-dependency checks, a bundle-size budget, secret scanning and SAST.

## Architecture

Discovery builds a resource catalog. Each subscribed GVR gets a dynamic informer whose cache is projected into table rows; a WebSocket sends one snapshot then deltas keyed by uid. Informers strip managedFields before caching. Exec and port-forwarding get their own transports: a binary WebSocket carrying the Kubernetes exec channel protocol, and a process-global forward registry independent of any request.

The React app is embedded with `embed.FS`. One HTTP and WebSocket server backs both the browser tab and the desktop window, and you move between them mid-session.

Built on client-go v0.36, so Kubernetes 1.35 to 1.37 by skew policy. Runs in production against k3s v1.36.
