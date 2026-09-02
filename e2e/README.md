# The browser suite

Drives the built binary against a real cluster in a real browser. Everything here exists because a
unit test cannot reach it: informer deltas arriving over a WebSocket and changing a rendered table,
Monaco and xterm and uPlot, the write path, and what spinoza says when something it needs is absent.

## Running it

```sh
just test-e2e        # the core tier: four nodes, metrics-server, a CRD, a fake Prometheus
just test-e2e-full   # six nodes, plus Flux, Argo CD and a scale fixture
just cluster-down    # delete the cluster either one made
```

Both bring their own cluster, build the binary and install Chromium. `SPINOZA_KIND_CLUSTER` picks
the cluster name, which is how several checkouts share one machine without colliding — the listen
ports are derived from it.

Both take an optional test name and an optional spec path, so a single test can be re-run without
sitting through the tier:

```sh
just test-e2e 'the view says what it is for'                              # by name, anywhere
just test-e2e '' specs/history.spec.ts                                    # one file
just test-e2e 'clearing the history' specs/history.spec.ts                # both
```

`just test-traffic-live [kubeconfig]` is the read-only check against a live cluster. It uses the
current kube context when the path is omitted, asks a real Prometheus for real Cilium Hubble
metrics, and requires at least one workload-to-workload edge. It does not create or change cluster
resources.

The name is a `--grep` pattern and the path is passed to Playwright as-is. The cluster is reused
when it is already up, so a second filtered run costs seconds. Mind that a spec written to run
after its neighbours may fail alone, or pass alone and fail in the tier — `history` reads what
earlier specs wrote. When a filtered run disagrees with a full one, the full one is right.

## Five spinozas, not one

Most specs drive the main instance. Four more start beside it, each crippled a different way, so the
degradation paths are testable rather than assumed:

| instance | started with | what it is for |
|---|---|---|
| `readonly` | a ServiceAccount kubeconfig with read-only RBAC | partial-data banners, forbidden listings, greyed-out actions |
| `toolless` | the same, plus `--helm` and `--kubectl` pointed at nothing | a button disabled instead of one that fails when pressed |
| `nowhere` | a kubeconfig whose server is `https://127.0.0.1:1` | no cluster, feed dropped, discovery failed, retry offered |
| `traffic` | `--prometheus e2e/fake-prom:9090` | the mesh graph, drawn from canned flow metrics |
| `profiled` | `--pprof` | the profiler mounted, behind the same token |

Spinoza exits five seconds after its last view closes and that grace is not configurable, so a spec
using a side instance holds a page open on it with `holdSide` for the length of the file.

## Writing a spec

**Read the tree before writing the selector.** Add a throwaway spec that prints
`ariaSnapshot()` of the surface, run it on the box, then write assertions against what actually
resolves. Reading the source instead is how three earlier passes went wrong. What it catches:

- the first `[role="tabpanel"]` on the page is the *bottom* dock, not the drawer
- the release detail's tabs are `aria-pressed` buttons, not tabs
- a row is selected by clicking the name button inside it, not the row
- `toContainText` joins DOM text with no spaces, so `Kubernetes v1.36.1` reads as `Kubernetesv1.36.1`

**Assert the artefact, not the chrome.** The topology spec used to check for the strings "Owns" and
"Routes to" — a hard-coded legend that renders whether or not an edge exists, which is why the suite
stayed green through a graph that drew none. Count `.react-flow__edge` and measure
`getTotalLength()`.

**Every degradation test needs a positive twin**, or a permanently broken feature passes as one
correctly reporting itself absent.

**A write asserts twice**: once on the screen, once with `kubectl` or `helm`.

**No `waitForTimeout` as synchronisation.** Settings persist through a debounced PUT, so a spec that
reloads must wait on the request, not on a sleep.

## Things that bite

- Monaco's textarea cannot be clicked — the rendered text intercepts the pointer. Click `.view-lines`
  then `.focus()` the textarea. It renders spaces as ` `, and both typing and `insertText` are
  subject to auto-indent, so a multi-line draft comes out over-indented. Insert single-line flow YAML.
- Panel layout and theme live on the server, shared by every browser context in the run. A spec that
  moves a panel puts it back in `afterAll`, which runs even when the spec fails.
- The URL hash needs `context=` before any `view=` is honoured.
- Write specs run before read specs alphabetically and recycle the pods the reads look at, so a read
  picks a *Running* row rather than the first one.

## Not covered

The Wails desktop window. Playwright cannot drive it. It is the same server and the same frontend as
browser mode, so the coverage transfers, but nothing here exercises the window itself.
