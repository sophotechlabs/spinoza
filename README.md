# Spinoza

[![go](https://img.shields.io/github/actions/workflow/status/sophotechlabs/spinoza/go.yaml?branch=main&label=go)](https://github.com/sophotechlabs/spinoza/actions/workflows/go.yaml)
[![frontend](https://img.shields.io/github/actions/workflow/status/sophotechlabs/spinoza/frontend.yaml?branch=main&label=frontend)](https://github.com/sophotechlabs/spinoza/actions/workflows/frontend.yaml)
[![repo](https://img.shields.io/github/actions/workflow/status/sophotechlabs/spinoza/repo.yaml?branch=main&label=repo)](https://github.com/sophotechlabs/spinoza/actions/workflows/repo.yaml)
[![go coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/sophotechlabs/spinoza/badges/coverage-go.json)](https://github.com/sophotechlabs/spinoza/actions/workflows/go.yaml)
[![web coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/sophotechlabs/spinoza/badges/coverage-web.json)](https://github.com/sophotechlabs/spinoza/actions/workflows/frontend.yaml)
[![license](https://img.shields.io/badge/license-FSL--1.1--ALv2-blue)](LICENSE)

A self-hosted Kubernetes GUI: Go backend with client-go informers, React frontend, one binary. Runs as a browser tab or a desktop window. [spinoza.tech](https://spinoza.tech)

**Source-available, not open source.** [FSL-1.1-ALv2](LICENSE): run it, modify it and redistribute it internally, for professional services, and for non-commercial education and research. You may not offer it as a commercial product or service that competes with it. Each release becomes Apache-2.0 two years after it ships.

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

Spinoza needs a kubeconfig. Helm releases are read straight from the cluster; upgrades, rollbacks and debug containers call `helm` and `kubectl`.

## What it does

Live clusters below, Borg theme.

### Every GVR discovery reports

One informer-backed view per type, so CRDs appear without per-type code. A snapshot, then deltas over a WebSocket keyed by uid. Filter by name or by any column of the kind in view, completed from what the cluster reported. Selecting a row fills the inspect drawer.

![Spinoza pods table with the inspect drawer open on coredns: ports, metadata, conditions, containers and owner references](docs/images/pods.png)

### GitOps, grouped by role

Flux and Argo CD side by side: cluster sync, controller health, and every applier and source with its ready count. Reconcile, suspend and resume Flux objects. Sync and refresh Argo Applications.

![Spinoza Flux overview: all systems operational, cluster sync from a GitRepository, controller health, and counts for appliers, sources and image automation](docs/images/flux-overview.png)

The dependency graph draws sources, Kustomizations and HelmReleases by what manages what. Argo's is the app-of-apps, with ApplicationSets above what they generate. Clicking a node opens it in the drawer with its actions.

![Spinoza GitOps dependency graph: sources, Kustomizations and HelmReleases with the edges that manage and depend on each other](docs/images/gitops-graph.png)

### Helm without the binary

Releases read straight from Helm's own storage objects, secrets or configmaps, whichever driver wrote them. Chart, app version, the newest version your repos offer, revision, status, values, notes, rendered manifest and history. OCI end to end. Upgrade, rollback and uninstall call `helm`, and an upgrade shows a server-rendered manifest diff before you confirm. A release owned by Flux gets no upgrade button; it links to its HelmRelease, because that change belongs in git.

![Spinoza Helm releases with chart and app versions, revision, status, and a Latest column flagging available upgrades](docs/images/helm-releases.png)

### Inspect and edit

Metadata, conditions, events, and live YAML edited against the cluster's own schema in Monaco with `monaco-yaml`. Server-side apply and delete. An edited draft survives a background reload.

![Spinoza deployments table with the inspect drawer open on live YAML in Monaco, with apply, revert and delete](docs/images/inspect-yaml.png)

### Port-forwarding

Any pod or service on localhost without touching kubectl. Forwards survive navigation, are listed in one place, and stop with one click.

![Spinoza pods table with the inspect drawer open on Grafana's ports: 3000 forwarded to a local port with open and stop controls, and a toast confirming the forward](docs/images/port-forward.png)

### Write actions that show their work

Scale, rollout restart, cordon, uncordon, drain. Drain plans first: it counts what it would evict, leave in place and block, names the reason for each, and keeps the button disabled until you accept the blocked pods. Destructive actions can be put behind a typed confirmation per cluster, which then asks for the object's name.

![Spinoza drain plan for a node: how many pods it would evict, leave in place and block, the reason for each, and the drain button disabled behind a checkbox](docs/images/drain-plan.png)

### The rest

- **Cluster overview**: server version, node readiness, allocatable CPU and memory against live usage, pods by phase, newest warning events.
- **Logs** per container, streamed. Pausing follow stops the scroll, not the stream.
- **Exec** into a container over `v5.channel.k8s.io` in a real terminal. Shell-less images get an ephemeral debug container instead, on a verdict cached per image digest.
- **Metrics**: live usage from metrics-server in the tables, CPU and memory history from Prometheus through the apiserver proxy. `--prometheus namespace/service:port` overrides discovery.
- **Nine themes**, contrast-gated in CI, plus your own as JSON.

## Running it

Spinoza starts on your current kubeconfig context, and the top bar switches between every context it can see. **Kubeconfigs** adds another file by path, referenced rather than copied or merged, so contexts stay grouped under the file they came from. The list lives in `kubeconfigs.json` in your user config directory. `--kubeconfig PATH` replaces the default lookup for one run.

Spinoza binds loopback only and rejects requests whose Host or Origin is not local. Every route and both WebSockets require the token generated for that run, sent as the `X-Spinoza-Token` header, a `token` query parameter, or the cookie the page keeps. `--token-file PATH` writes it out at mode 0600. When the last view closes, the process exits.

## Develop

Toolchain versions are pinned in `mise.toml` (Go 1.26, Node 24); `mise install` gets them.

```sh
just deps      # frontend dependencies, once
just run       # build the frontend and the binary, then start
just dev-api   # Go server on :34115
just dev-web   # Vite, proxying to the API
just rund      # build and open the desktop app
```

Vite serves its own `index.html` without the token, so open `http://localhost:5173/?token=<the token dev-api printed>`.

The binary embeds the built frontend. `just stub-assets` writes a placeholder for the Go-only recipes.

Every check is a `just` recipe: `just check` before pushing, `just ci` for everything CI runs, which adds cross-compilation, the coverage gate, `govulncheck`, dead-code and unused-dependency checks, a bundle-size budget, secret scanning and SAST.

## Architecture

Discovery builds a resource catalog. Each subscribed GVR gets a dynamic informer whose cache is projected into table rows; a WebSocket sends one snapshot and then deltas keyed by uid. Informers strip managedFields before caching. Exec and port-forwarding get their own transports: a binary WebSocket carrying the Kubernetes exec channel protocol, and a process-global forward registry independent of any request.

The React app is embedded with `embed.FS`. One HTTP and WebSocket server backs both the browser tab and the desktop window, and you can move between them mid-session.

Built on client-go v0.36, so Kubernetes 1.35 to 1.37 by skew policy. Runs in production against k3s v1.36.

## License

Copyright 2026 Sophotech s.r.o. Source-available under the [Functional Source License](LICENSE) (FSL-1.1-ALv2), not open source.

Run it, modify it, redistribute it: internally, for professional services, for non-commercial education and research. You may not offer it to others as a commercial product or service that competes with it. Each release becomes Apache-2.0 two years after it ships.
