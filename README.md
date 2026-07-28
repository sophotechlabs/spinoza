# Spinoza

A fast, lightweight, open-source Kubernetes GUI. Go backend with client-go informers + React frontend, shipped as a single binary. Native window via Wails is planned; the current build serves a browser tab.

**Status: POC (Phase 0).** A walking skeleton — one live resource (pods) streamed over WebSocket into a Lens-style UI. Pod data is stub data until the informer lands (Track A). The Lens layout is minimal until Track C.

## Run

Prereqs: Go 1.26+, Node 20+.

```sh
just deps    # once — install frontend deps
just run     # build frontend + binary, start server
```

Then open `http://127.0.0.1:34115`.

## Develop (hot reload, two terminals)

```sh
just dev-api   # Go server on :34115 (stub data)
just dev-web   # Vite dev server, proxies /ws to the API
```

## CI

CI runs on the self-hosted Forgejo forge, not GitHub. Workflows live in `.forgejo/workflows/` and stay dormant until the repo is pushed there:

```sh
git remote add ci git@git.c.p-mk1.sopho.tech:arch/spinoza.git
git push ci main
```

Every check is a `just` recipe, so the same command runs on a laptop and in CI. `just ci` runs the whole suite; `just check` is the fast pre-push subset that `lefthook` already calls.

Runs: `https://git.c.p-mk1.sopho.tech/arch/spinoza/actions`

## Architecture

Single Go binary: `client-go` informer → in-process broker → HTTP+WS server that streams a snapshot then deltas. The React SPA (embedded via `embed.FS`) applies them to a uid-keyed store and renders the table. The same HTTP+WS transport runs in a browser tab now and inside a Wails window later — no rewrite.

License: Apache-2.0 (planned). Contributions under DCO sign-off.
