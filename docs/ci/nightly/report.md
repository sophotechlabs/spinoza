# Nightly 2026-11-21 — regression

Commit `bf36dd3be4cd`, triggered by schedule. [Run](https://github.com/sophotechlabs/spinoza/actions/runs/44892771949)

| | this run | |
|---|---|---|
| jobs | 95 | 32 failed |
| job-minutes | 722.7 (-14.9) | |
| e2e coverage | 47% (-0.1) | |
| mutation score | 100% (no change) | 7824 killed, 0 lived |
| flaky | 1 (+1) | passed only on retry |
| uncovered mutants | 341 (no change) | records across reported build variants |

Mutation score is killed / (killed + lived). Uncovered mutants are excluded from that score; some are uninstrumented constant expressions.

## Separate system coverage

These suites retain their own Go statement denominators. The browser E2E coverage above is unchanged by these profiles.

| suite | covered / total statements | coverage |
|---|---|---|
| cluster-mode-auth | 4926 / 21386 | 23% |
| cluster-mode-chromium | 3862 / 21386 | 18.1% |
| cluster-mode-firefox | 3865 / 21386 | 18.1% |
| cluster-mode-webkit | 3859 / 21386 | 18% |
| mcp-cli | 794 / 21580 | 3.7% |

## New failures

- install / windows amd64, binary and PATH

## Failing

- e2e / checks-issues-worklist / webkit
- e2e / foundation-security / webkit
- e2e / gitops / chromium
- e2e / gitops / firefox
- e2e / gitops / webkit
- e2e / helm / webkit
- e2e / inspect-compare-rbac-topology / chromium
- e2e / inspect-compare-rbac-topology / firefox
- e2e / inspect-compare-rbac-topology / webkit
- e2e / multicluster-fleet / chromium
- e2e / multicluster-fleet / firefox
- e2e / multicluster-fleet / webkit
- e2e / mutations-protection-history / webkit
- e2e / navigation-interaction / chromium
- e2e / navigation-interaction / firefox
- e2e / navigation-interaction / webkit
- e2e / observability-traffic / webkit
- e2e / outage / webkit
- e2e / resilience-capacity-soak / chromium
- e2e / resilience-capacity-soak / firefox
- e2e / resilience-capacity-soak / webkit
- e2e / resources-live-tables / chromium
- e2e / resources-live-tables / firefox
- e2e / resources-live-tables / webkit
- e2e / streams-terminals-forwards / chromium
- e2e / streams-terminals-forwards / firefox
- e2e / streams-terminals-forwards / webkit
- e2e / visual-accessibility / chromium
- e2e / visual-accessibility / firefox
- e2e / visual-accessibility / webkit
- install / windows amd64, binary and PATH
- install / windows arm64, binary and PATH

## Flaky

| test | file | project | attempts |
|---|---|---|---|
| a drain preview classifies every live pod without touching the node | specs/actions.spec.ts:240 | firefox | 2 |

## Jobs

| job | result | minutes |
|---|---|---|
| cluster-mode-release | success | 3.2 |
| e2e / checks-issues-worklist / chromium | success | 8.1 |
| e2e / checks-issues-worklist / firefox | success | 8 |
| e2e / checks-issues-worklist / webkit | failure | 5.6 |
| e2e / cluster mode / chromium | success | 7.2 |
| e2e / cluster mode / firefox | success | 6.4 |
| e2e / cluster mode / webkit | success | 8 |
| e2e / cluster-mode-auth | success | 12.3 |
| e2e / distribution-desktop-install | success | 0.1 |
| e2e / e2e-coverage | success | 0.2 |
| e2e / foundation-security / chromium | success | 4.1 |
| e2e / foundation-security / firefox | success | 3.7 |
| e2e / foundation-security / webkit | failure | 6.3 |
| e2e / gitops / chromium | failure | 7.8 |
| e2e / gitops / firefox | failure | 8 |
| e2e / gitops / webkit | failure | 8.1 |
| e2e / helm / chromium | success | 4.8 |
| e2e / helm / firefox | success | 4.7 |
| e2e / helm / webkit | failure | 4.9 |
| e2e / inspect-compare-rbac-topology / chromium | failure | 7.7 |
| e2e / inspect-compare-rbac-topology / firefox | failure | 7.4 |
| e2e / inspect-compare-rbac-topology / webkit | failure | 7.8 |
| e2e / mcp-cli | success | 3.8 |
| e2e / multicluster-fleet / chromium | failure | 7.8 |
| e2e / multicluster-fleet / firefox | failure | 7.9 |
| e2e / multicluster-fleet / webkit | failure | 8.1 |
| e2e / mutations-protection-history / chromium | success | 5.2 |
| e2e / mutations-protection-history / firefox | success | 4.7 |
| e2e / mutations-protection-history / webkit | failure | 6 |
| e2e / navigation-interaction / chromium | failure | 7.4 |
| e2e / navigation-interaction / firefox | failure | 7.7 |
| e2e / navigation-interaction / webkit | failure | 8 |
| e2e / observability-traffic / chromium | success | 3.9 |
| e2e / observability-traffic / firefox | success | 4.1 |
| e2e / observability-traffic / webkit | failure | 5.8 |
| e2e / outage / chromium | success | 3.7 |
| e2e / outage / firefox | success | 4 |
| e2e / outage / webkit | failure | 5 |
| e2e / resilience-capacity-soak / chromium | failure | 4.8 |
| e2e / resilience-capacity-soak / firefox | failure | 7.6 |
| e2e / resilience-capacity-soak / webkit | failure | 8.1 |
| e2e / resources-live-tables / chromium | failure | 7.8 |
| e2e / resources-live-tables / firefox | failure | 7.9 |
| e2e / resources-live-tables / webkit | failure | 8.3 |
| e2e / select | success | 0.1 |
| e2e / streams-terminals-forwards / chromium | failure | 7.9 |
| e2e / streams-terminals-forwards / firefox | failure | 8.6 |
| e2e / streams-terminals-forwards / webkit | failure | 6.7 |
| e2e / suite-contract | success | 0.4 |
| e2e / visual-accessibility / chromium | failure | 7.8 |
| e2e / visual-accessibility / firefox | failure | 7.7 |
| e2e / visual-accessibility / webkit | failure | 13 |
| fuzz / fuzz (., FuzzServingCheckPublicURL) | success | 11.1 |
| fuzz / fuzz (./internal/access, FuzzDecisionAggregation) | success | 10.6 |
| fuzz / fuzz (./internal/auth, FuzzRoleAuthorization) | success | 10.3 |
| fuzz / fuzz (./internal/charts, FuzzChartIndex) | success | 10.3 |
| fuzz / fuzz (./internal/charts, FuzzFetchableRepositoryURL) | success | 10.4 |
| fuzz / fuzz (./internal/checks, FuzzParseRules) | success | 10.4 |
| fuzz / fuzz (./internal/issues, FuzzCursor) | success | 10.3 |
| fuzz / fuzz (./internal/mcp, FuzzProtocol) | success | 10.4 |
| fuzz / fuzz (./internal/mcp, FuzzStdioFraming) | success | 10.3 |
| fuzz / fuzz (./internal/store, FuzzHistoryLimit) | success | 10.7 |
| fuzz / fuzz (./internal/store, FuzzTimelineCellsRoundTrip) | success | 10.7 |
| install / alpine, busybox wget | success | 0.2 |
| install / alpine, curl on musl | success | 0.2 |
| install / debian, curl on glibc | success | 0.3 |
| install / debian, no downloader | success | 0.2 |
| install / macos, binary and app | success | 0.2 |
| install / windows amd64, binary and PATH | failure | 0.6 |
| install / windows arm64, binary and PATH | failure | 1 |
| mutation / mutation (checks-a-e) | success | 13.9 |
| mutation / mutation (checks-f-j) | success | 13.6 |
| mutation / mutation (checks-k-o) | success | 7.9 |
| mutation / mutation (checks-p-r) | success | 8.9 |
| mutation / mutation (checks-s-t) | success | 9.4 |
| mutation / mutation (checks-u-z) | success | 5.5 |
| mutation / mutation (cmd) | success | 0.5 |
| mutation / mutation (internal-a-d) | success | 15.3 |
| mutation / mutation (internal-e-l) | success | 31 |
| mutation / mutation (internal-m-r) | success | 16.9 |
| mutation / mutation (internal-s-z) | success | 6 |
| mutation / mutation (resources-a-f) | success | 4.3 |
| mutation / mutation (resources-g-l) | success | 1 |
| mutation / mutation (resources-m-r) | success | 11.8 |
| mutation / mutation (resources-s-z) | success | 4.6 |
| mutation / mutation (root-default) | success | 5.3 |
| mutation / mutation (root-desktop) | success | 6.2 |
| mutation / mutation (server-a-c) | success | 24.7 |
| mutation / mutation (server-d-e) | success | 4.9 |
| mutation / mutation (server-f) | success | 15.5 |
| mutation / mutation (server-g-l) | success | 21.3 |
| mutation / mutation (server-m-r) | success | 9.8 |
| mutation / mutation (server-s-z) | success | 35 |
| mutation / mutation-total | success | 0.3 |
| repeat | success | 6.7 |
