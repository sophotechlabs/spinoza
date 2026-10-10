# Nightly 2026-10-10 — green

Commit `84db3f3aff5c`, triggered by schedule. [Run](https://github.com/sophotechlabs/spinoza/actions/runs/38039453936)

| | this run | |
|---|---|---|
| jobs | 95 | 0 failed |
| job-minutes | 819.6 (-30.1) | |
| e2e coverage | 64.9% (-0.2) | |
| mutation score | 100% (no change) | 7809 killed, 0 lived |
| flaky | 0 (-1) | passed only on retry |
| uncovered mutants | 341 (no change) | records across reported build variants |

Mutation score is killed / (killed + lived). Uncovered mutants are excluded from that score; some are uninstrumented constant expressions.

## Separate system coverage

These suites retain their own Go statement denominators. The browser E2E coverage above is unchanged by these profiles.

| suite | covered / total statements | coverage |
|---|---|---|
| cluster-mode-auth | 5245 / 22027 | 23.8% |
| cluster-mode-chromium | 4112 / 22027 | 18.7% |
| cluster-mode-firefox | 4115 / 22027 | 18.7% |
| cluster-mode-webkit | 4115 / 22027 | 18.7% |
| mcp-cli | 833 / 22188 | 3.8% |

## Fixed since the last run

- fuzz / fuzz (./internal/charts, FuzzChartIndex)

## Jobs

| job | result | minutes |
|---|---|---|
| cluster-mode-release | success | 3 |
| e2e / checks-issues-worklist / chromium | success | 8.8 |
| e2e / checks-issues-worklist / firefox | success | 8.9 |
| e2e / checks-issues-worklist / webkit | success | 8.2 |
| e2e / cluster mode / chromium | success | 6.7 |
| e2e / cluster mode / firefox | success | 6.2 |
| e2e / cluster mode / webkit | success | 7.1 |
| e2e / cluster-mode-auth | success | 10.8 |
| e2e / distribution-desktop-install | success | 0.1 |
| e2e / e2e-coverage | success | 0.3 |
| e2e / foundation-security / chromium | success | 5 |
| e2e / foundation-security / firefox | success | 4.1 |
| e2e / foundation-security / webkit | success | 5.4 |
| e2e / gitops / chromium | success | 10.4 |
| e2e / gitops / firefox | success | 11.9 |
| e2e / gitops / webkit | success | 12.8 |
| e2e / helm / chromium | success | 5.4 |
| e2e / helm / firefox | success | 5.8 |
| e2e / helm / webkit | success | 6.1 |
| e2e / inspect-compare-rbac-topology / chromium | success | 10.2 |
| e2e / inspect-compare-rbac-topology / firefox | success | 10.9 |
| e2e / inspect-compare-rbac-topology / webkit | success | 12.1 |
| e2e / mcp-cli | success | 3.1 |
| e2e / multicluster-fleet / chromium | success | 10.7 |
| e2e / multicluster-fleet / firefox | success | 9.5 |
| e2e / multicluster-fleet / webkit | success | 10.3 |
| e2e / mutations-protection-history / chromium | success | 5.5 |
| e2e / mutations-protection-history / firefox | success | 5.7 |
| e2e / mutations-protection-history / webkit | success | 6.7 |
| e2e / navigation-interaction / chromium | success | 11.9 |
| e2e / navigation-interaction / firefox | success | 10.5 |
| e2e / navigation-interaction / webkit | success | 13.8 |
| e2e / observability-traffic / chromium | success | 5.2 |
| e2e / observability-traffic / firefox | success | 4.6 |
| e2e / observability-traffic / webkit | success | 5.7 |
| e2e / outage / chromium | success | 4.9 |
| e2e / outage / firefox | success | 5 |
| e2e / outage / webkit | success | 4.8 |
| e2e / resilience-capacity-soak / chromium | success | 10.4 |
| e2e / resilience-capacity-soak / firefox | success | 9.8 |
| e2e / resilience-capacity-soak / webkit | success | 9.5 |
| e2e / resources-live-tables / chromium | success | 10 |
| e2e / resources-live-tables / firefox | success | 9.6 |
| e2e / resources-live-tables / webkit | success | 11 |
| e2e / select | success | 0.2 |
| e2e / streams-terminals-forwards / chromium | success | 6.1 |
| e2e / streams-terminals-forwards / firefox | success | 6.9 |
| e2e / streams-terminals-forwards / webkit | success | 6.9 |
| e2e / suite-contract | success | 0.3 |
| e2e / visual-accessibility / chromium | success | 15.2 |
| e2e / visual-accessibility / firefox | success | 15.3 |
| e2e / visual-accessibility / webkit | success | 15.9 |
| fuzz / fuzz (., FuzzServingCheckPublicURL) | success | 12.7 |
| fuzz / fuzz (./internal/access, FuzzDecisionAggregation) | success | 11.3 |
| fuzz / fuzz (./internal/auth, FuzzRoleAuthorization) | success | 10.7 |
| fuzz / fuzz (./internal/charts, FuzzChartIndex) | success | 10.6 |
| fuzz / fuzz (./internal/charts, FuzzFetchableRepositoryURL) | success | 10.6 |
| fuzz / fuzz (./internal/checks, FuzzParseRules) | success | 11.5 |
| fuzz / fuzz (./internal/issues, FuzzCursor) | success | 10.8 |
| fuzz / fuzz (./internal/mcp, FuzzProtocol) | success | 11.3 |
| fuzz / fuzz (./internal/mcp, FuzzStdioFraming) | success | 11.8 |
| fuzz / fuzz (./internal/store, FuzzHistoryLimit) | success | 11 |
| fuzz / fuzz (./internal/store, FuzzTimelineCellsRoundTrip) | success | 10.7 |
| install / alpine, busybox wget | success | 0.2 |
| install / alpine, curl on musl | success | 0.2 |
| install / debian, curl on glibc | success | 0.4 |
| install / debian, no downloader | success | 0.2 |
| install / macos, binary and app | success | 0.3 |
| install / windows amd64, binary and PATH | success | 0.5 |
| install / windows arm64, binary and PATH | success | 0.9 |
| mutation / mutation (checks-a-e) | success | 9.5 |
| mutation / mutation (checks-f-j) | success | 16.3 |
| mutation / mutation (checks-k-o) | success | 6.9 |
| mutation / mutation (checks-p-r) | success | 13.3 |
| mutation / mutation (checks-s-t) | success | 7.4 |
| mutation / mutation (checks-u-z) | success | 6.3 |
| mutation / mutation (cmd) | success | 2.6 |
| mutation / mutation (internal-a-d) | success | 17.2 |
| mutation / mutation (internal-e-l) | success | 22.9 |
| mutation / mutation (internal-m-r) | success | 16.3 |
| mutation / mutation (internal-s-z) | success | 6.3 |
| mutation / mutation (resources-a-f) | success | 6.9 |
| mutation / mutation (resources-g-l) | success | 2.4 |
| mutation / mutation (resources-m-r) | success | 12.5 |
| mutation / mutation (resources-s-z) | success | 5.3 |
| mutation / mutation (root-default) | success | 6.2 |
| mutation / mutation (root-desktop) | success | 7.2 |
| mutation / mutation (server-a-c) | success | 24.4 |
| mutation / mutation (server-d-e) | success | 8.3 |
| mutation / mutation (server-f) | success | 22.5 |
| mutation / mutation (server-g-l) | success | 23.3 |
| mutation / mutation (server-m-r) | success | 12 |
| mutation / mutation (server-s-z) | success | 27.7 |
| mutation / mutation-total | success | 0.1 |
| repeat | success | 4.9 |
