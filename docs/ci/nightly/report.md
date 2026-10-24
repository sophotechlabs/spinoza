# Nightly 2026-10-24 — green

Commit `da05424b93c3`, triggered by schedule. [Run](https://github.com/sophotechlabs/spinoza/actions/runs/40074616268)

| | this run | |
|---|---|---|
| jobs | 95 | 0 failed |
| job-minutes | 754.6 (-7.8) | |
| e2e coverage | 65% (-0.1) | |
| mutation score | 100% (no change) | 7824 killed, 0 lived |
| flaky | 1 (no change) | passed only on retry |
| uncovered mutants | 341 (no change) | records across reported build variants |

Mutation score is killed / (killed + lived). Uncovered mutants are excluded from that score; some are uninstrumented constant expressions.

## Separate system coverage

These suites retain their own Go statement denominators. The browser E2E coverage above is unchanged by these profiles.

| suite | covered / total statements | coverage |
|---|---|---|
| cluster-mode-auth | 5250 / 22065 | 23.8% |
| cluster-mode-chromium | 4119 / 22065 | 18.7% |
| cluster-mode-firefox | 4120 / 22065 | 18.7% |
| cluster-mode-webkit | 4114 / 22065 | 18.6% |
| mcp-cli | 833 / 22226 | 3.7% |

## Flaky

| test | file | project | attempts |
|---|---|---|---|
| the resource tree counts what it found and remembers its expanded category | specs/navigation.spec.ts:265 | webkit | 2 |

## Jobs

| job | result | minutes |
|---|---|---|
| cluster-mode-release | success | 2.9 |
| e2e / checks-issues-worklist / chromium | success | 7.5 |
| e2e / checks-issues-worklist / firefox | success | 7.1 |
| e2e / checks-issues-worklist / webkit | success | 7.3 |
| e2e / cluster mode / chromium | success | 6.3 |
| e2e / cluster mode / firefox | success | 5.9 |
| e2e / cluster mode / webkit | success | 7 |
| e2e / cluster-mode-auth | success | 10.9 |
| e2e / distribution-desktop-install | success | 0.1 |
| e2e / e2e-coverage | success | 0.3 |
| e2e / foundation-security / chromium | success | 5.1 |
| e2e / foundation-security / firefox | success | 3.7 |
| e2e / foundation-security / webkit | success | 3.9 |
| e2e / gitops / chromium | success | 9.8 |
| e2e / gitops / firefox | success | 10.6 |
| e2e / gitops / webkit | success | 10.6 |
| e2e / helm / chromium | success | 4.8 |
| e2e / helm / firefox | success | 4 |
| e2e / helm / webkit | success | 3.9 |
| e2e / inspect-compare-rbac-topology / chromium | success | 9.6 |
| e2e / inspect-compare-rbac-topology / firefox | success | 10.6 |
| e2e / inspect-compare-rbac-topology / webkit | success | 11.7 |
| e2e / mcp-cli | success | 3.2 |
| e2e / multicluster-fleet / chromium | success | 9.5 |
| e2e / multicluster-fleet / firefox | success | 9.5 |
| e2e / multicluster-fleet / webkit | success | 9.2 |
| e2e / mutations-protection-history / chromium | success | 4.3 |
| e2e / mutations-protection-history / firefox | success | 4.9 |
| e2e / mutations-protection-history / webkit | success | 5.3 |
| e2e / navigation-interaction / chromium | success | 11.3 |
| e2e / navigation-interaction / firefox | success | 11 |
| e2e / navigation-interaction / webkit | success | 12 |
| e2e / observability-traffic / chromium | success | 4.9 |
| e2e / observability-traffic / firefox | success | 3.7 |
| e2e / observability-traffic / webkit | success | 3.8 |
| e2e / outage / chromium | success | 3.7 |
| e2e / outage / firefox | success | 3.6 |
| e2e / outage / webkit | success | 4.3 |
| e2e / resilience-capacity-soak / chromium | success | 8.4 |
| e2e / resilience-capacity-soak / firefox | success | 9 |
| e2e / resilience-capacity-soak / webkit | success | 8.4 |
| e2e / resources-live-tables / chromium | success | 8.8 |
| e2e / resources-live-tables / firefox | success | 8.8 |
| e2e / resources-live-tables / webkit | success | 9.5 |
| e2e / select | success | 0.2 |
| e2e / streams-terminals-forwards / chromium | success | 5.1 |
| e2e / streams-terminals-forwards / firefox | success | 5.8 |
| e2e / streams-terminals-forwards / webkit | success | 5.8 |
| e2e / suite-contract | success | 0.4 |
| e2e / visual-accessibility / chromium | success | 13.8 |
| e2e / visual-accessibility / firefox | success | 14 |
| e2e / visual-accessibility / webkit | success | 13.6 |
| fuzz / fuzz (., FuzzServingCheckPublicURL) | success | 10.9 |
| fuzz / fuzz (./internal/access, FuzzDecisionAggregation) | success | 10.3 |
| fuzz / fuzz (./internal/auth, FuzzRoleAuthorization) | success | 10.4 |
| fuzz / fuzz (./internal/charts, FuzzChartIndex) | success | 10.4 |
| fuzz / fuzz (./internal/charts, FuzzFetchableRepositoryURL) | success | 10.3 |
| fuzz / fuzz (./internal/checks, FuzzParseRules) | success | 10.6 |
| fuzz / fuzz (./internal/issues, FuzzCursor) | success | 10.3 |
| fuzz / fuzz (./internal/mcp, FuzzProtocol) | success | 10.7 |
| fuzz / fuzz (./internal/mcp, FuzzStdioFraming) | success | 10.6 |
| fuzz / fuzz (./internal/store, FuzzHistoryLimit) | success | 10.8 |
| fuzz / fuzz (./internal/store, FuzzTimelineCellsRoundTrip) | success | 10.8 |
| install / alpine, busybox wget | success | 0.3 |
| install / alpine, curl on musl | success | 0.4 |
| install / debian, curl on glibc | success | 0.3 |
| install / debian, no downloader | success | 0.2 |
| install / macos, binary and app | success | 0.3 |
| install / windows amd64, binary and PATH | success | 0.5 |
| install / windows arm64, binary and PATH | success | 1 |
| mutation / mutation (checks-a-e) | success | 13.8 |
| mutation / mutation (checks-f-j) | success | 12.9 |
| mutation / mutation (checks-k-o) | success | 9.1 |
| mutation / mutation (checks-p-r) | success | 9.9 |
| mutation / mutation (checks-s-t) | success | 9.3 |
| mutation / mutation (checks-u-z) | success | 5.2 |
| mutation / mutation (cmd) | success | 0.9 |
| mutation / mutation (internal-a-d) | success | 13.4 |
| mutation / mutation (internal-e-l) | success | 27.7 |
| mutation / mutation (internal-m-r) | success | 15.6 |
| mutation / mutation (internal-s-z) | success | 7.2 |
| mutation / mutation (resources-a-f) | success | 5.5 |
| mutation / mutation (resources-g-l) | success | 1.1 |
| mutation / mutation (resources-m-r) | success | 13.6 |
| mutation / mutation (resources-s-z) | success | 4.6 |
| mutation / mutation (root-default) | success | 5.3 |
| mutation / mutation (root-desktop) | success | 4.8 |
| mutation / mutation (server-a-c) | success | 19.4 |
| mutation / mutation (server-d-e) | success | 6.3 |
| mutation / mutation (server-f) | success | 19.9 |
| mutation / mutation (server-g-l) | success | 27.7 |
| mutation / mutation (server-m-r) | success | 11 |
| mutation / mutation (server-s-z) | success | 25.2 |
| mutation / mutation-total | success | 0.2 |
| repeat | success | 6.5 |
