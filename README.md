# Spinoza

A self-hosted Kubernetes GUI. Go backend with client-go informers, React frontend, one binary. Runs as a browser tab or a Wails desktop window.

## Run

Prereqs: Go 1.26+, Node 20+, a kubeconfig.

```sh
just deps    # once — install frontend deps
just run     # build frontend + binary, start the server
```

Then open `http://127.0.0.1:34115`. `just rund` builds and opens the desktop app instead.

Spinoza uses your current kubeconfig context. It listens on loopback only and refuses requests whose Host or Origin is not local, because the process holds full cluster credentials.

## What it does

- **Browse every resource type** discovery reports, grouped into a Lens-style tree. One generic informer-backed view per GVR, so CRDs show up without any per-type code.
- **GitOps view** for Flux and Argo: dependency graph, per-kind lists, a reporting-status overview. Reconcile, suspend and resume Flux objects.
- **Inspect drawer**: metadata, conditions, events, live YAML with schema-aware editing (Monaco + `monaco-yaml`), server-side apply and delete.
- **Write actions**: scale, rollout restart, cordon, uncordon, and drain — drain plans first and shows what it would evict, skip and refuse before you confirm it.
- **Logs** streamed per container into the bottom dock.
- **Exec** into a container over `v5.channel.k8s.io` into a real terminal (xterm). Distroless images have no shell; spinoza probes once and caches the verdict per image digest.
- **Ephemeral debug containers** by wrapping `kubectl debug` for the images with no shell. Needs `kubectl` on PATH.
- **Port-forward** to a pod or service, listed and stoppable from one place, surviving navigation.
- **Metrics**: current usage from metrics-server in the tables, CPU and memory history charted from Prometheus through the apiserver proxy. `--prometheus namespace/service:port` overrides discovery.

## Develop

```sh
just dev-api   # Go server on :34115
just dev-web   # Vite dev server, proxies to the API
```

## CI

CI runs on a self-hosted Forgejo forge. Workflows live in `.forgejo/workflows/`.

```sh
git remote add ci ssh://git@git.c.p-mk1.sopho.tech:2222/arch/spinoza.git
git push ci main
```

Every check is a `just` recipe, so the same command runs on a laptop and in CI. `just ci` runs everything; `just check` is the pre-push subset that `lefthook` calls. CI adds cross-compilation, the coverage gate, `govulncheck`, dead-code and unused-dependency checks, a bundle-size budget, secret scanning and SAST.

Runs: `https://git.c.p-mk1.sopho.tech/arch/spinoza/actions`

## Architecture

Discovery builds a resource catalog; each subscribed GVR gets a dynamic informer whose cache is projected into table rows. A WebSocket sends a snapshot then deltas, keyed by uid. Exec and port-forward do not fit that pipe — they are long-lived and bidirectional, so they get their own transports: a binary WebSocket carrying the Kubernetes exec channel protocol, and a process-global forward registry independent of any request.

The React SPA is embedded with `embed.FS`. The same HTTP+WS server backs both the browser tab and the Wails window.

Contributions under DCO sign-off.
