# Nightly 2026-10-03 — green

Commit `ce599108b14f`, triggered by schedule. [Run](https://github.com/sophotechlabs/spinoza/actions/runs/37109956033)

| | this run | |
|---|---|---|
| jobs | 95 | 0 failed |
| job-minutes | 829.7 (-5.1) | |
| e2e coverage | 65.1% (+0.1) | |
| mutation score | 100% (no change) | 7809 killed, 0 lived |
| flaky | 3 (+3) | passed only on retry |
| uncovered mutants | 341 (no change) | records across reported build variants |

Mutation score is killed / (killed + lived). Uncovered mutants are excluded from that score; some are uninstrumented constant expressions.

## Separate system coverage

These suites retain their own Go statement denominators. The browser E2E coverage above is unchanged by these profiles.

| suite | covered / total statements | coverage |
|---|---|---|
| cluster-mode-auth | 5245 / 22027 | 23.8% |
| cluster-mode-chromium | 4118 / 22027 | 18.7% |
| cluster-mode-firefox | 4115 / 22027 | 18.7% |
| cluster-mode-webkit | 4113 / 22027 | 18.7% |
| mcp-cli | 833 / 22188 | 3.8% |

## Flaky

| test | file | project | attempts |
|---|---|---|---|
| at 200% browser zoom › the whole workspace stays reachable by scrolling | specs-full/viewport.spec.ts:89 | webkit | 2 |
| one live connection accepts 64 subscriptions and refuses the 65th | specs/capacity.spec.ts:11 | webkit | 2 |
| the palette opens on its shortcut and closes on escape | specs/palette.spec.ts:7 | webkit | 2 |

## Jobs

| job | result | minutes |
|---|---|---|
| cluster-mode-release | success | 3.2 |
| e2e / checks-issues-worklist / chromium | success | 8.4 |
| e2e / checks-issues-worklist / firefox | success | 8.9 |
| e2e / checks-issues-worklist / webkit | success | 9.7 |
| e2e / cluster mode / chromium | success | 6.8 |
| e2e / cluster mode / firefox | success | 5.6 |
| e2e / cluster mode / webkit | success | 7.1 |
| e2e / cluster-mode-auth | success | 9.2 |
| e2e / distribution-desktop-install | success | 0.1 |
| e2e / e2e-coverage | success | 0.3 |
| e2e / foundation-security / chromium | success | 4.9 |
| e2e / foundation-security / firefox | success | 4.8 |
| e2e / foundation-security / webkit | success | 4.5 |
| e2e / gitops / chromium | success | 10.7 |
| e2e / gitops / firefox | success | 11.4 |
| e2e / gitops / webkit | success | 11.5 |
| e2e / helm / chromium | success | 5.6 |
| e2e / helm / firefox | success | 4.8 |
| e2e / helm / webkit | success | 5.1 |
| e2e / inspect-compare-rbac-topology / chromium | success | 9.7 |
| e2e / inspect-compare-rbac-topology / firefox | success | 11.5 |
| e2e / inspect-compare-rbac-topology / webkit | success | 10.5 |
| e2e / mcp-cli | success | 3.3 |
| e2e / multicluster-fleet / chromium | success | 9.6 |
| e2e / multicluster-fleet / firefox | success | 9.8 |
| e2e / multicluster-fleet / webkit | success | 10.6 |
| e2e / mutations-protection-history / chromium | success | 4.6 |
| e2e / mutations-protection-history / firefox | success | 6 |
| e2e / mutations-protection-history / webkit | success | 6.2 |
| e2e / navigation-interaction / chromium | success | 10.8 |
| e2e / navigation-interaction / firefox | success | 12.2 |
| e2e / navigation-interaction / webkit | success | 13.1 |
| e2e / observability-traffic / chromium | success | 5.1 |
| e2e / observability-traffic / firefox | success | 4.9 |
| e2e / observability-traffic / webkit | success | 5.4 |
| e2e / outage / chromium | success | 4.3 |
| e2e / outage / firefox | success | 4.7 |
| e2e / outage / webkit | success | 4.7 |
| e2e / resilience-capacity-soak / chromium | success | 9.9 |
| e2e / resilience-capacity-soak / firefox | success | 9.6 |
| e2e / resilience-capacity-soak / webkit | success | 10.2 |
| e2e / resources-live-tables / chromium | success | 9.6 |
| e2e / resources-live-tables / firefox | success | 10.4 |
| e2e / resources-live-tables / webkit | success | 10.7 |
| e2e / select | success | 0.2 |
| e2e / streams-terminals-forwards / chromium | success | 5.9 |
| e2e / streams-terminals-forwards / firefox | success | 7.3 |
| e2e / streams-terminals-forwards / webkit | success | 6.8 |
| e2e / suite-contract | success | 0.3 |
| e2e / visual-accessibility / chromium | success | 14.9 |
| e2e / visual-accessibility / firefox | success | 15.1 |
| e2e / visual-accessibility / webkit | success | 13.7 |
| fuzz / fuzz (., FuzzServingCheckPublicURL) | success | 12.5 |
| fuzz / fuzz (./internal/access, FuzzDecisionAggregation) | success | 11.3 |
| fuzz / fuzz (./internal/auth, FuzzRoleAuthorization) | success | 10.8 |
| fuzz / fuzz (./internal/charts, FuzzChartIndex) | success | 10.7 |
| fuzz / fuzz (./internal/charts, FuzzFetchableRepositoryURL) | success | 10.6 |
| fuzz / fuzz (./internal/checks, FuzzParseRules) | success | 11.5 |
| fuzz / fuzz (./internal/issues, FuzzCursor) | success | 10.8 |
| fuzz / fuzz (./internal/mcp, FuzzProtocol) | success | 11.9 |
| fuzz / fuzz (./internal/mcp, FuzzStdioFraming) | success | 12 |
| fuzz / fuzz (./internal/store, FuzzHistoryLimit) | success | 11 |
| fuzz / fuzz (./internal/store, FuzzTimelineCellsRoundTrip) | success | 10.9 |
| install / alpine, busybox wget | success | 0.3 |
| install / alpine, curl on musl | success | 0.2 |
| install / debian, curl on glibc | success | 0.4 |
| install / debian, no downloader | success | 0.2 |
| install / macos, binary and app | success | 0.3 |
| install / windows amd64, binary and PATH | success | 0.7 |
| install / windows arm64, binary and PATH | success | 0.9 |
| mutation / mutation (checks-a-e) | success | 14.6 |
| mutation / mutation (checks-f-j) | success | 13 |
| mutation / mutation (checks-k-o) | success | 8.2 |
| mutation / mutation (checks-p-r) | success | 13.3 |
| mutation / mutation (checks-s-t) | success | 10.2 |
| mutation / mutation (checks-u-z) | success | 6.1 |
| mutation / mutation (cmd) | success | 2.3 |
| mutation / mutation (internal-a-d) | success | 17.6 |
| mutation / mutation (internal-e-l) | success | 34.7 |
| mutation / mutation (internal-m-r) | success | 16.5 |
| mutation / mutation (internal-s-z) | success | 9.4 |
| mutation / mutation (resources-a-f) | success | 7 |
| mutation / mutation (resources-g-l) | success | 2.2 |
| mutation / mutation (resources-m-r) | success | 14.7 |
| mutation / mutation (resources-s-z) | success | 4.1 |
| mutation / mutation (root-default) | success | 7.3 |
| mutation / mutation (root-desktop) | success | 5.4 |
| mutation / mutation (server-a-c) | success | 16.9 |
| mutation / mutation (server-d-e) | success | 8.2 |
| mutation / mutation (server-f) | success | 22.5 |
| mutation / mutation (server-g-l) | success | 23 |
| mutation / mutation (server-m-r) | success | 12.1 |
| mutation / mutation (server-s-z) | success | 32.6 |
| mutation / mutation-total | success | 0.2 |
| repeat | success | 6.4 |
