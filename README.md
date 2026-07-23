# Spinoza

A fast, lightweight, open-source Kubernetes GUI. Go backend with client-go informers + React frontend, shipped as a single binary. Native window via Wails is planned; the current build serves a browser tab.

**Status: POC (Phase 0).** A walking skeleton — one live resource (pods) streamed over WebSocket into a Lens-style UI. Pod data is stub data until the informer lands (Track A). The Lens layout is minimal until Track C.

## Run

Prereqs: Go 1.26+, Node 20+.

```
make deps    # once — install frontend deps
make run     # build frontend + binary, start server
```

Then open `http://127.0.0.1:34115`.

## Develop (hot reload, two terminals)

```
make dev-api   # Go server on :34115 (stub data)
make dev-web   # Vite dev server, proxies /ws to the API
```

## Architecture

Single Go binary: `client-go` informer → in-process broker → HTTP+WS server that streams a snapshot then deltas. The React SPA (embedded via `embed.FS`) applies them to a uid-keyed store and renders the table. The same HTTP+WS transport runs in a browser tab now and inside a Wails window later — no rewrite.

License: Apache-2.0 (planned). Contributions under DCO sign-off.
