# Nightly 2026-09-18 — green

Commit `f9a47eedc0ee`, triggered by schedule. [Run](https://github.com/sophotechlabs/spinoza/actions/runs/35320116388)

| | this run | |
|---|---|---|
| jobs | 95 | 0 failed |
| job-minutes | 794.4 (+9) | |
| e2e coverage | 60.3% (-0.1) | |
| mutation score | 100% (no change) | 7783 killed, 0 lived |
| flaky | 0 (-1) | passed only on retry |

## Jobs

| job | result | minutes |
|---|---|---|
| cluster-mode-release | success | 3.3 |
| e2e / checks-issues-worklist / chromium | success | 5.4 |
| e2e / checks-issues-worklist / firefox | success | 5.5 |
| e2e / checks-issues-worklist / webkit | success | 6.2 |
| e2e / cluster mode / chromium | success | 5.7 |
| e2e / cluster mode / firefox | success | 5.6 |
| e2e / cluster mode / webkit | success | 7.1 |
| e2e / cluster-mode-auth | success | 10.6 |
| e2e / distribution-desktop-install | success | 0.1 |
| e2e / e2e-coverage | success | 0.3 |
| e2e / foundation-security / chromium | success | 4.7 |
| e2e / foundation-security / firefox | success | 4.8 |
| e2e / foundation-security / webkit | success | 5.1 |
| e2e / gitops / chromium | success | 8.1 |
| e2e / gitops / firefox | success | 8.5 |
| e2e / gitops / webkit | success | 8.7 |
| e2e / helm / chromium | success | 4.7 |
| e2e / helm / firefox | success | 5.6 |
| e2e / helm / webkit | success | 5.4 |
| e2e / inspect-compare-rbac-topology / chromium | success | 8.9 |
| e2e / inspect-compare-rbac-topology / firefox | success | 9.5 |
| e2e / inspect-compare-rbac-topology / webkit | success | 10.6 |
| e2e / mcp-cli | success | 3.2 |
| e2e / multicluster-fleet / chromium | success | 9 |
| e2e / multicluster-fleet / firefox | success | 9.2 |
| e2e / multicluster-fleet / webkit | success | 9.2 |
| e2e / mutations-protection-history / chromium | success | 5.2 |
| e2e / mutations-protection-history / firefox | success | 5.6 |
| e2e / mutations-protection-history / webkit | success | 6.2 |
| e2e / navigation-interaction / chromium | success | 10.4 |
| e2e / navigation-interaction / firefox | success | 11.1 |
| e2e / navigation-interaction / webkit | success | 11.1 |
| e2e / observability-traffic / chromium | success | 4.5 |
| e2e / observability-traffic / firefox | success | 4.8 |
| e2e / observability-traffic / webkit | success | 4.9 |
| e2e / outage / chromium | success | 4.8 |
| e2e / outage / firefox | success | 5.4 |
| e2e / outage / webkit | success | 4.9 |
| e2e / resilience-capacity-soak / chromium | success | 7.6 |
| e2e / resilience-capacity-soak / firefox | success | 7.6 |
| e2e / resilience-capacity-soak / webkit | success | 7.9 |
| e2e / resources-live-tables / chromium | success | 8.6 |
| e2e / resources-live-tables / firefox | success | 8.9 |
| e2e / resources-live-tables / webkit | success | 9.2 |
| e2e / select | success | 0.1 |
| e2e / streams-terminals-forwards / chromium | success | 5.2 |
| e2e / streams-terminals-forwards / firefox | success | 5.5 |
| e2e / streams-terminals-forwards / webkit | success | 5.6 |
| e2e / suite-contract | success | 0.3 |
| e2e / visual-accessibility / chromium | success | 13 |
| e2e / visual-accessibility / firefox | success | 13.9 |
| e2e / visual-accessibility / webkit | success | 13.4 |
| fuzz / fuzz (., FuzzServingCheckPublicURL) | success | 12 |
| fuzz / fuzz (./internal/access, FuzzDecisionAggregation) | success | 12 |
| fuzz / fuzz (./internal/auth, FuzzRoleAuthorization) | success | 10.7 |
| fuzz / fuzz (./internal/charts, FuzzChartIndex) | success | 10.7 |
| fuzz / fuzz (./internal/charts, FuzzFetchableRepositoryURL) | success | 10.7 |
| fuzz / fuzz (./internal/checks, FuzzParseRules) | success | 11.5 |
| fuzz / fuzz (./internal/issues, FuzzCursor) | success | 10.7 |
| fuzz / fuzz (./internal/mcp, FuzzProtocol) | success | 11.8 |
| fuzz / fuzz (./internal/mcp, FuzzStdioFraming) | success | 11.3 |
| fuzz / fuzz (./internal/store, FuzzHistoryLimit) | success | 10.9 |
| fuzz / fuzz (./internal/store, FuzzTimelineCellsRoundTrip) | success | 11 |
| install / alpine, busybox wget | success | 0.2 |
| install / alpine, curl on musl | success | 0.2 |
| install / debian, curl on glibc | success | 0.4 |
| install / debian, no downloader | success | 0.2 |
| install / macos, binary and app | success | 0.3 |
| install / windows amd64, binary and PATH | success | 0.6 |
| install / windows arm64, binary and PATH | success | 1 |
| mutation / mutation (checks-a-e) | success | 14.8 |
| mutation / mutation (checks-f-j) | success | 13.6 |
| mutation / mutation (checks-k-o) | success | 7.5 |
| mutation / mutation (checks-p-r) | success | 10.8 |
| mutation / mutation (checks-s-t) | success | 9.9 |
| mutation / mutation (checks-u-z) | success | 6.1 |
| mutation / mutation (cmd) | success | 2.6 |
| mutation / mutation (internal-a-d) | success | 18.1 |
| mutation / mutation (internal-e-l) | success | 35.9 |
| mutation / mutation (internal-m-r) | success | 16.7 |
| mutation / mutation (internal-s-z) | success | 7.7 |
| mutation / mutation (resources-a-f) | success | 6.6 |
| mutation / mutation (resources-g-l) | success | 2.7 |
| mutation / mutation (resources-m-r) | success | 14.6 |
| mutation / mutation (resources-s-z) | success | 5 |
| mutation / mutation (root-default) | success | 7 |
| mutation / mutation (root-desktop) | success | 6.9 |
| mutation / mutation (server-a-c) | success | 24 |
| mutation / mutation (server-d-e) | success | 6.8 |
| mutation / mutation (server-f) | success | 21.9 |
| mutation / mutation (server-g-l) | success | 30.7 |
| mutation / mutation (server-m-r) | success | 10.2 |
| mutation / mutation (server-s-z) | success | 31.1 |
| mutation / mutation-total | success | 0.1 |
| repeat | success | 6.2 |
