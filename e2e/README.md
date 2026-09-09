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

## Which groups a change runs

`suite.json` maps the repository onto seventeen capability groups, and `scripts/select-groups.mjs`
picks the groups a diff needs. Per changed file, in order:

1. `unitOnlyPaths` — Go unit tests and testdata inside the production trees. They cannot change
    the binary, so they select nothing beyond the smoke group.
2. `fullRunPaths` — cross-cutting code: the harness, the router, the WebSocket, the stores, the
    shared frontend libraries. One such file runs every group.
3. A group's `paths` — the file selects that group. A file may belong to several.
4. Anything else under `productionRoots` is production code no group owns. It runs every group,
    and `scripts/validate-suite.mjs` refuses to let one be committed.

The smoke group always runs. `fullRunBudget` is how many production files sit in `fullRunPaths`;
the validator fails when the number grows, so a new file gets an owning group instead of a free
pass, and says when the budget can come down. The mapping was derived from the import graph on
2026-09-07: a frontend file whose importers span five or more groups, or that `App` or `Root`
import directly, is cross-cutting; a server handler belongs to the groups whose packages it
calls. To see what a diff would run:

```sh
node e2e/scripts/select-groups.mjs --files changed.txt
```

The stderr line names the file that forced a full run, and CI's step summary repeats it.

## Which tier a group runs on

Selection says *which* groups a diff needs. The tier says *when* they run and on how many browsers.

Each group carries a `tier`, either `commit` or `nightly`, and an `observedMinutes` measured from a
real run. `commitBrowsers` is chromium alone; `nightlyBrowsers` is all three. A push runs the selected
commit-tier groups on chromium; the nightly runs every group on every browser, whatever the diff.

```sh
node e2e/scripts/select-groups.mjs --files changed.txt --tier commit
node e2e/scripts/select-groups.mjs --all --tier nightly
```

`commitBudgetMinutes` is what the commit tier is allowed to cost, and `validate-suite.mjs` refuses a
tier assignment that exceeds it, saying by how much and how much room is left. A Playwright group
costs `observedMinutes` per browser; a cluster-mode group costs its own job plus one browser job each,
which is what `browserMinutes` is for. So a new group is `nightly` and free until someone decides it
is worth part of the per-commit budget, and moving one to `commit` fails the gate unless another
leaves. The same shape one level up governs whole workflows, in `.github/ci-tiers.json`.

## Coverage and flakes

The binary the suite drives is built with `-cover`, and every instance writes its counters to
`e2e/.tmp/cover/<instance>` when it exits. `just e2e-cover` merges them into `e2e/coverage.out`
and prints per-package percentages. CI uploads each job's counters, and a final job merges every
selected group's into one number, marked partial when the run was selective or a job failed.
This is what the browser suite exercises, as distinct from what the unit tests exercise; the two
profiles are kept apart on purpose. An instance that has to be killed leaves a `<name>.unclean`
marker and the merge refuses to report rather than report less than it should. A binary reused
through `SPINOZA_E2E_SKIP_BUILD=1` that was built without `-cover` writes no counters, and the
merge says so instead of reporting zero.

In CI Playwright also writes `test-results/report.json`, and `just e2e-flaky` lists the tests
that passed only on retry in the step summary. The count is informational until a baseline exists.

## Not covered

The Wails desktop window. Playwright cannot drive it. It is the same server and the same frontend as
browser mode, so the coverage transfers, but nothing here exercises the window itself.
