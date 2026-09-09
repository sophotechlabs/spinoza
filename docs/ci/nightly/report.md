# Nightly 2026-09-09 — green

Commit `e527121584b5`, triggered by push. [Run](https://github.com/sophotechlabs/spinoza/actions/runs/34397514372)

| | this run | |
|---|---|---|
| jobs | 87 | 0 failed |
| job-minutes | 681.9 | |
| e2e coverage | 61.5% | |
| mutation score | 100% | 6984 killed, 0 lived |
| flaky | 0 | passed only on retry |

## Jobs

| job | result | minutes |
|---|---|---|
| e2e / checks-issues-worklist / chromium | success | 4.2 |
| e2e / checks-issues-worklist / firefox | success | 4.2 |
| e2e / checks-issues-worklist / webkit | success | 4.4 |
| e2e / cluster mode / chromium | success | 5.5 |
| e2e / cluster mode / firefox | success | 6.4 |
| e2e / cluster mode / webkit | success | 6.7 |
| e2e / cluster-mode-auth | success | 10 |
| e2e / distribution-desktop-install | success | 0.1 |
| e2e / e2e-coverage | success | 0.3 |
| e2e / foundation-security / chromium | success | 3.6 |
| e2e / foundation-security / firefox | success | 3.8 |
| e2e / foundation-security / webkit | success | 4.7 |
| e2e / gitops / chromium | success | 7.5 |
| e2e / gitops / firefox | success | 6.7 |
| e2e / gitops / webkit | success | 7.5 |
| e2e / helm / chromium | success | 4 |
| e2e / helm / firefox | success | 3.8 |
| e2e / helm / webkit | success | 5.8 |
| e2e / inspect-compare-rbac-topology / chromium | success | 7.3 |
| e2e / inspect-compare-rbac-topology / firefox | success | 7.5 |
| e2e / inspect-compare-rbac-topology / webkit | success | 10.2 |
| e2e / mcp-cli | success | 3.3 |
| e2e / multicluster-fleet / chromium | success | 7.2 |
| e2e / multicluster-fleet / firefox | success | 7.8 |
| e2e / multicluster-fleet / webkit | success | 7.5 |
| e2e / mutations-protection-history / chromium | success | 3.9 |
| e2e / mutations-protection-history / firefox | success | 4.5 |
| e2e / mutations-protection-history / webkit | success | 4.8 |
| e2e / navigation-interaction / chromium | success | 9.1 |
| e2e / navigation-interaction / firefox | success | 9.6 |
| e2e / navigation-interaction / webkit | success | 12.9 |
| e2e / observability-traffic / chromium | success | 3.4 |
| e2e / observability-traffic / firefox | success | 3.7 |
| e2e / observability-traffic / webkit | success | 3.8 |
| e2e / outage / chromium | success | 4 |
| e2e / outage / firefox | success | 4.1 |
| e2e / outage / webkit | success | 3.8 |
| e2e / resilience-capacity-soak / chromium | success | 7.3 |
| e2e / resilience-capacity-soak / firefox | success | 9.2 |
| e2e / resilience-capacity-soak / webkit | success | 6.7 |
| e2e / resources-live-tables / chromium | success | 7.3 |
| e2e / resources-live-tables / firefox | success | 7.2 |
| e2e / resources-live-tables / webkit | success | 8.2 |
| e2e / select | success | 0.2 |
| e2e / streams-terminals-forwards / chromium | success | 3.9 |
| e2e / streams-terminals-forwards / firefox | success | 4.2 |
| e2e / streams-terminals-forwards / webkit | success | 4.4 |
| e2e / suite-contract | success | 0.3 |
| e2e / visual-accessibility / chromium | success | 12.1 |
| e2e / visual-accessibility / firefox | success | 12.3 |
| e2e / visual-accessibility / webkit | success | 12.8 |
| fuzz / fuzz (., FuzzServingCheckPublicURL) | success | 11.3 |
| fuzz / fuzz (./internal/access, FuzzDecisionAggregation) | success | 10.3 |
| fuzz / fuzz (./internal/auth, FuzzRoleAuthorization) | success | 10.2 |
| fuzz / fuzz (./internal/charts, FuzzChartIndex) | success | 10.3 |
| fuzz / fuzz (./internal/charts, FuzzFetchableRepositoryURL) | success | 10.3 |
| fuzz / fuzz (./internal/checks, FuzzParseRules) | success | 10.6 |
| fuzz / fuzz (./internal/issues, FuzzCursor) | success | 10.3 |
| fuzz / fuzz (./internal/mcp, FuzzProtocol) | success | 10.5 |
| fuzz / fuzz (./internal/mcp, FuzzStdioFraming) | success | 10.5 |
| fuzz / fuzz (./internal/store, FuzzHistoryLimit) | success | 10.8 |
| fuzz / fuzz (./internal/store, FuzzTimelineCellsRoundTrip) | success | 10.7 |
| mutation / mutation (checks-a-e) | success | 12.2 |
| mutation / mutation (checks-f-j) | success | 15.2 |
| mutation / mutation (checks-k-o) | success | 9.1 |
| mutation / mutation (checks-p-r) | success | 13.1 |
| mutation / mutation (checks-s-t) | success | 7.5 |
| mutation / mutation (checks-u-z) | success | 5.3 |
| mutation / mutation (cmd) | success | 0.9 |
| mutation / mutation (internal-a-d) | success | 16 |
| mutation / mutation (internal-e-l) | success | 34.6 |
| mutation / mutation (internal-m-r) | success | 15.2 |
| mutation / mutation (internal-s-z) | success | 5.4 |
| mutation / mutation (resources-a-f) | success | 5.1 |
| mutation / mutation (resources-g-l) | success | 1.2 |
| mutation / mutation (resources-m-r) | success | 12.7 |
| mutation / mutation (resources-s-z) | success | 3.7 |
| mutation / mutation (root-default) | success | 3.5 |
| mutation / mutation (root-desktop) | success | 4.8 |
| mutation / mutation (server-a-c) | success | 16.2 |
| mutation / mutation (server-d-e) | success | 5.2 |
| mutation / mutation (server-f) | success | 18.5 |
| mutation / mutation (server-g-l) | success | 20.7 |
| mutation / mutation (server-m-r) | success | 8.1 |
| mutation / mutation (server-s-z) | success | 22.2 |
| mutation / mutation-total | success | 0.1 |
| repeat | success | 5.9 |
