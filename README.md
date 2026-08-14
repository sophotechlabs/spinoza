# Spinoza

A self-hosted Kubernetes GUI. Go backend with client-go informers, React frontend, one binary. Runs as a browser tab or a Wails desktop window.

## Run

Prereqs: Go 1.26+, Node 20+, a kubeconfig.

```sh
just deps    # once — install frontend deps
just run     # build frontend + binary, start the server
```

Open the URL it prints on start — it carries a token generated for that run, and the page keeps it in a cookie so a reload still works. `just rund` builds and opens the desktop app instead.

Spinoza starts on your current kubeconfig context and the top bar switches between every context it can see. **Kubeconfigs** next to that dropdown adds another file by path — the desktop app opens a file dialog for it. An added file is only ever referenced: spinoza re-reads it on every listing and every switch, never copies it, and never merges it with your default one, so contexts stay grouped under the file they came from. The list of files lives in `kubeconfigs.json` under your user config directory; `--kubeconfig PATH` still replaces the default lookup for the run.

It refuses to start on a non-loopback address, and refuses requests whose Host or Origin is not local, because the process holds full cluster credentials. Loopback is not a permission boundary, so every route and both WebSockets also require this run's token, as the `X-Spinoza-Token` header, a `token` query parameter or the cookie. `--token-file PATH` writes it out (mode 0600) for scripts.

## What it does

- **Cluster overview**: server version, node readiness, allocatable CPU and memory against live usage, pods by phase, and the most recent warning events.
- **Browse every resource type** discovery reports, grouped into a Lens-style tree. One generic informer-backed view per GVR, so CRDs show up without any per-type code.
- **Helm releases** read straight out of Helm's own storage objects — secrets or configmaps, whichever driver wrote them. Chart, app version, the newest version your configured repos offer, revision, status and age; open one for its values, notes, rendered manifest, resources and revision history. Reading needs no helm binary; **upgrade, rollback and uninstall** shell out to `helm` and say so when it is missing. An upgrade picks its version from your configured repos, starts from the values you supplied (never `--reuse-values`), and shows a server-rendered manifest diff before you confirm — the server render needs helm 3.13+. A release owned by Flux gets no upgrade button; it links to its HelmRelease object instead, because that change belongs in git.
- **GitOps view** for Flux and Argo: an overview grouped by role, a dependency graph, per-kind lists and a status-tile board. Reconcile, suspend and resume Flux objects.
- **Inspect drawer**: metadata, conditions, events, live YAML with schema-aware editing (Monaco + `monaco-yaml`), server-side apply and delete. Collapsible, and an edited YAML draft survives a background reload.
- **Write actions**: scale, rollout restart, cordon, uncordon, and drain — drain plans first and shows what it would evict, skip and refuse before you confirm it.
- **Logs** streamed per container in the pod's inspect drawer. Pausing follow stops the scroll, not the stream.
- **Exec** into a container over `v5.channel.k8s.io` into a real terminal (xterm). Distroless images have no shell; spinoza probes once and caches the verdict per image digest.
- **Ephemeral debug containers** by wrapping `kubectl debug` for the images with no shell. Needs `kubectl` on PATH.
- **Port-forward** to a pod or service, listed and stoppable from one place, surviving navigation.
- **Metrics**: current usage from metrics-server in the tables, CPU and memory history charted from Prometheus through the apiserver proxy. `--prometheus namespace/service:port` overrides discovery.

## Develop

```sh
just dev-api   # Go server on :34115
just dev-web   # Vite dev server, proxies to the API
```

Vite serves its own `index.html`, so it cannot inject the token: open `http://localhost:5173/?token=<the token dev-api printed>` and the app picks it up from the URL.

The binary embeds the built frontend, so `go build` on its own fails with `pattern web/dist/index.html: no matching files found` until there is one. `just build` produces the real thing; `just stub-assets` drops in a placeholder page, which is what the Go-only recipes and `just dev-api` use so they need no Node.

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

## License

Copyright 2026 Sophotech s.r.o. Source-available under the [Functional Source License](LICENSE) (FSL-1.1-ALv2), not open source.

Run it, modify it, redistribute it — internally, for professional services, for non-commercial education and research. You may not offer it to others as a commercial product or service that competes with it. Each release becomes Apache-2.0 two years after it ships.
