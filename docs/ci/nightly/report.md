# Nightly 2026-09-16 — regression

Commit `496023f4c792`, triggered by schedule. [Run](https://github.com/sophotechlabs/spinoza/actions/runs/35070770746)

| | this run | |
|---|---|---|
| jobs | 87 | 1 failed |
| job-minutes | 769.5 (+87.6) | |
| e2e coverage | 60.6% (-0.9) | |
| mutation score | 100% (no change) | 7783 killed, 0 lived |
| flaky | 2 (+2) | passed only on retry |

## New failures

- e2e / inspect-compare-rbac-topology / firefox

## Failing

- e2e / inspect-compare-rbac-topology / firefox

## Flaky

| test | file | project | attempts |
|---|---|---|---|
| at 200% browser zoom › the whole workspace stays reachable by scrolling | specs-full/viewport.spec.ts:89 | webkit | 2 |
| at the supported floor › the chrome a person steers with is all on screen | specs-full/viewport.spec.ts:63 | webkit | 2 |

## Jobs

| job | result | minutes |
|---|---|---|
| e2e / checks-issues-worklist / chromium | success | 5.3 |
| e2e / checks-issues-worklist / firefox | success | 6 |
| e2e / checks-issues-worklist / webkit | success | 5.8 |
| e2e / cluster mode / chromium | success | 6 |
| e2e / cluster mode / firefox | success | 6.3 |
| e2e / cluster mode / webkit | success | 8.4 |
| e2e / cluster-mode-auth | success | 9.7 |
| e2e / distribution-desktop-install | success | 0.2 |
| e2e / e2e-coverage | success | 0.4 |
| e2e / foundation-security / chromium | success | 4.8 |
| e2e / foundation-security / firefox | success | 4.6 |
| e2e / foundation-security / webkit | success | 5.1 |
| e2e / gitops / chromium | success | 8.6 |
| e2e / gitops / firefox | success | 8.6 |
| e2e / gitops / webkit | success | 8.5 |
| e2e / helm / chromium | success | 5.2 |
| e2e / helm / firefox | success | 5.2 |
| e2e / helm / webkit | success | 5.8 |
| e2e / inspect-compare-rbac-topology / chromium | success | 9 |
| e2e / inspect-compare-rbac-topology / firefox | failure | 9.3 |
| e2e / inspect-compare-rbac-topology / webkit | success | 9.7 |
| e2e / mcp-cli | success | 3.2 |
| e2e / multicluster-fleet / chromium | success | 8.9 |
| e2e / multicluster-fleet / firefox | success | 8.8 |
| e2e / multicluster-fleet / webkit | success | 11.7 |
| e2e / mutations-protection-history / chromium | success | 5 |
| e2e / mutations-protection-history / firefox | success | 5.4 |
| e2e / mutations-protection-history / webkit | success | 5.7 |
| e2e / navigation-interaction / chromium | success | 10.4 |
| e2e / navigation-interaction / firefox | success | 10.9 |
| e2e / navigation-interaction / webkit | success | 11.3 |
| e2e / observability-traffic / chromium | success | 4.8 |
| e2e / observability-traffic / firefox | success | 4.9 |
| e2e / observability-traffic / webkit | success | 4.9 |
| e2e / outage / chromium | success | 4.7 |
| e2e / outage / firefox | success | 4.9 |
| e2e / outage / webkit | success | 5 |
| e2e / resilience-capacity-soak / chromium | success | 7.5 |
| e2e / resilience-capacity-soak / firefox | success | 7.9 |
| e2e / resilience-capacity-soak / webkit | success | 8.2 |
| e2e / resources-live-tables / chromium | success | 7.8 |
| e2e / resources-live-tables / firefox | success | 8.6 |
| e2e / resources-live-tables / webkit | success | 9 |
| e2e / select | success | 0.2 |
| e2e / streams-terminals-forwards / chromium | success | 5.2 |
| e2e / streams-terminals-forwards / firefox | success | 5.7 |
| e2e / streams-terminals-forwards / webkit | success | 5.7 |
| e2e / suite-contract | success | 0.4 |
| e2e / visual-accessibility / chromium | success | 13.7 |
| e2e / visual-accessibility / firefox | success | 13.5 |
| e2e / visual-accessibility / webkit | success | 14.1 |
| fuzz / fuzz (., FuzzServingCheckPublicURL) | success | 12.2 |
| fuzz / fuzz (./internal/access, FuzzDecisionAggregation) | success | 11.6 |
| fuzz / fuzz (./internal/auth, FuzzRoleAuthorization) | success | 10.8 |
| fuzz / fuzz (./internal/charts, FuzzChartIndex) | success | 10.8 |
| fuzz / fuzz (./internal/charts, FuzzFetchableRepositoryURL) | success | 10.7 |
| fuzz / fuzz (./internal/checks, FuzzParseRules) | success | 11.5 |
| fuzz / fuzz (./internal/issues, FuzzCursor) | success | 10.8 |
| fuzz / fuzz (./internal/mcp, FuzzProtocol) | success | 11.9 |
| fuzz / fuzz (./internal/mcp, FuzzStdioFraming) | success | 11.7 |
| fuzz / fuzz (./internal/store, FuzzHistoryLimit) | success | 10.9 |
| fuzz / fuzz (./internal/store, FuzzTimelineCellsRoundTrip) | success | 10.8 |
| mutation / mutation (checks-a-e) | success | 15.5 |
| mutation / mutation (checks-f-j) | success | 12.6 |
| mutation / mutation (checks-k-o) | success | 8 |
| mutation / mutation (checks-p-r) | success | 11.1 |
| mutation / mutation (checks-s-t) | success | 10.3 |
| mutation / mutation (checks-u-z) | success | 5.1 |
| mutation / mutation (cmd) | success | 2.8 |
| mutation / mutation (internal-a-d) | success | 17.8 |
| mutation / mutation (internal-e-l) | success | 28.6 |
| mutation / mutation (internal-m-r) | success | 14.3 |
| mutation / mutation (internal-s-z) | success | 9.2 |
| mutation / mutation (resources-a-f) | success | 6.7 |
| mutation / mutation (resources-g-l) | success | 2.8 |
| mutation / mutation (resources-m-r) | success | 14.1 |
| mutation / mutation (resources-s-z) | success | 5.2 |
| mutation / mutation (root-default) | success | 7.3 |
| mutation / mutation (root-desktop) | success | 6.2 |
| mutation / mutation (server-a-c) | success | 19.3 |
| mutation / mutation (server-d-e) | success | 6.2 |
| mutation / mutation (server-f) | success | 18 |
| mutation / mutation (server-g-l) | success | 28.2 |
| mutation / mutation (server-m-r) | success | 12.1 |
| mutation / mutation (server-s-z) | success | 27.6 |
| mutation / mutation-total | success | 0.2 |
| repeat | success | 6.1 |
