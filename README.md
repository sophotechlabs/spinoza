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
curl -fsSL https://spinoza.tech/download | sh
```

The binary goes to `~/.local/bin` (or `/usr/local/bin` as root; `SPINOZA_INSTALL_DIR` overrides both), and the checksum is verified before anything is written. On macOS the desktop app is installed to `/Applications` as well, so Spotlight finds it.

```sh
spinoza --open          # browser
open -a Spinoza         # desktop window, macOS
```

Or take a tarball from [releases](https://github.com/sophotechlabs/spinoza/releases) for Linux, macOS or Windows. `SPINOZA_VERSION=v1.5.0` pins the installer to a version.

Spinoza needs a kubeconfig and nothing else. Reading Helm releases needs no `helm` binary; upgrades, rollbacks and debug containers shell out to `helm` and `kubectl` and say so when they are missing.

## Screenshots

Every shot is a live cluster, Borg theme.

**Flux, grouped by role.** Cluster sync from the repository, controller health, and every applier and source with its ready count.

![Spinoza Flux overview: all systems operational, cluster sync from a GitRepository, controller health, and counts for appliers, sources and image automation](docs/images/flux-overview.png)

**The dependency graph.** Sources, Kustomizations and HelmReleases, laid out by what manages what. Click a node to open it.

![Spinoza GitOps dependency graph: sources, Kustomizations and HelmReleases with the edges that manage and depend on each other](docs/images/gitops-graph.png)

**Pods.** Container health, restarts, node, live CPU and memory. Filter by name or by `field:value`.

![Spinoza pods table: every pod with container health, status, restarts, node, live CPU and memory, and age](docs/images/pods.png)

**Cluster overview.** Version, node readiness, allocatable capacity against live usage, and the newest warning events.

![Spinoza cluster overview: Kubernetes version, node and pod tiles, allocatable CPU and memory against live usage, and the newest warning events](docs/images/cluster-overview.png)

## What it does

- **Every resource type discovery reports**, grouped into a Lens-style tree. One informer-backed view per GVR, so CRDs appear without per-type code.
- **Cluster overview**: server version, node readiness, allocatable CPU and memory against live usage, pods by phase, newest warning events.
- **Helm releases** read straight from Helm's own storage objects, secrets or configmaps, whichever driver wrote them. Chart, app version, the newest version your repos offer, revision, status, values, notes, rendered manifest and history. Upgrade, rollback and uninstall shell out to `helm`; an upgrade shows a server-rendered manifest diff before you confirm. A release owned by Flux links to its HelmRelease instead, because that change belongs in git.
- **GitOps** for Flux and Argo CD: an overview grouped by role, a dependency graph, per-kind lists. Reconcile, suspend and resume Flux objects.
- **Inspect drawer** with metadata, conditions, events, and live YAML edited against the cluster's own schema (Monaco with `monaco-yaml`), server-side apply and delete. An edited draft survives a background reload.
- **Write actions**: scale, rollout restart, cordon, uncordon, drain. Drain plans first and shows what it would evict, skip and refuse.
- **Logs** per container, streamed. Pausing follow stops the scroll, not the stream.
- **Exec** into a container over `v5.channel.k8s.io` in a real terminal. Distroless images have no shell, so spinoza probes once and caches the verdict per image digest, then offers an **ephemeral debug container** instead.
- **Port-forwards** to pods or services, listed and stoppable in one place, surviving navigation.
- **Metrics**: live usage from metrics-server in the tables, CPU and memory history from Prometheus through the apiserver proxy. `--prometheus namespace/service:port` overrides discovery.
- **Filtering** by chips: `ns:`, `name:`, or any column of the kind you are looking at, with completion from what the cluster reported.

## Running it

Spinoza starts on your current kubeconfig context, and the top bar switches between every context it can see. **Kubeconfigs** adds another file by path: spinoza only ever references it, re-reading on every listing and switch, never copying or merging it, so contexts stay grouped under the file they came from. The list lives in `kubeconfigs.json` in your user config directory. `--kubeconfig PATH` replaces the default lookup for one run.

The process holds full cluster credentials, so it refuses to start on a non-loopback address and refuses requests whose Host or Origin is not local. Loopback is not a permission boundary on a shared machine, so every route and both WebSockets also require the token generated for that run, sent as the `X-Spinoza-Token` header, a `token` query parameter, or the cookie the page keeps. `--token-file PATH` writes it out at mode 0600 for scripts.

Destructive actions can be put behind a typed confirmation per cluster: with protection on, delete and apply ask you to type the object's name first.

## Develop

Toolchain versions are pinned in `mise.toml` (Go 1.26, Node 24); `mise install` gets them.

```sh
just deps      # frontend dependencies, once
just run       # build the frontend and the binary, then start
just dev-api   # Go server on :34115
just dev-web   # Vite, proxying to the API
just rund      # build and open the desktop app
```

Vite serves its own `index.html` and cannot inject the token, so open `http://localhost:5173/?token=<the token dev-api printed>`.

The binary embeds the built frontend, so a bare `go build` fails until one exists. `just stub-assets` drops in a placeholder, which is what the Go-only recipes use so they need no Node.

Every check is a `just` recipe, so the same command runs on a laptop and in CI: `just check` before pushing, `just ci` for everything CI does, which adds cross-compilation, the coverage gate, `govulncheck`, dead-code and unused-dependency checks, a bundle-size budget, secret scanning and SAST.

## Architecture

Discovery builds a resource catalog. Each subscribed GVR gets a dynamic informer whose cache is projected into table rows; a WebSocket sends one snapshot and then deltas keyed by uid. Exec and port-forwarding do not fit that pipe, being long-lived and bidirectional, so they get their own transports: a binary WebSocket carrying the Kubernetes exec channel protocol, and a process-global forward registry independent of any request.

The React app is embedded with `embed.FS`. The same HTTP and WebSocket server backs both the browser tab and the desktop window.
