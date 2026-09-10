# spinoza

Serves one Kubernetes cluster to a team from inside it: spinoza as a pod, behind
your ingress, with your identity provider deciding who gets in.

The guide is [docs/cluster-mode.md](../../../docs/cluster-mode.md). This file is
the values reference.

```sh
helm upgrade --install spinoza oci://ghcr.io/sophotechlabs/charts/spinoza \
  --namespace spinoza --create-namespace \
  --set publicURL=https://spinoza.example.com
```

`publicURL` is required. Set it to the HTTP(S) origin browsers use, without a
path, query, fragment, or credentials. A trailing `/` is accepted. The chart
refuses to render without it, and Spinoza refuses invalid values at startup.

## Values

| key | default | what it does |
|---|---|---|
| `publicURL` | — | The HTTP(S) origin browsers use. Required. |
| `unsafeAllowHTTP` | `false` | Allow a non-loopback plaintext browser origin. Exposes sessions and data to interception. |
| `replicaCount` | `1` | Has to be one; anything else is refused. See the guide. |
| `image.repository` | `ghcr.io/sophotechlabs/spinoza` | |
| `image.tag` | chart `appVersion` | |
| `impersonate` | `false` | Act on the cluster as the signed-in user. Requires an explicit impersonation scope. |
| `logLevel` | `info` | `debug`, `info`, `warn` or `error`. |
| `logFormat` | `json` | `json` or `text`. A log pipeline parses `json`. |
| `recordSessions` | `false` | Keep a transcript of every exec and node shell beside its audit entry. Admins read them. |
| `prometheus` | `""` | `namespace/service:port` for metric history; discovered when empty. |
| `nodeShell` | `false` | Allow a root shell on a node, which creates a privileged pod. |
| `auth.mode` | `""` | Required: `none`, `proxy` or `oidc`. |
| `auth.allowAnonymous` | `false` | Required unsafe opt-in when `auth.mode` is `none`. |
| `auth.defaultRole` | `viewer` | Role for anyone matching none of the group lists. |
| `auth.adminGroups` | `[]` | Groups whose members are admins here. |
| `auth.editorGroups` | `[]` | Groups whose members may change objects. |
| `auth.viewerGroups` | `[]` | Groups whose members may only look. |
| `auth.sessionTTL` | `8h` | How long a sign-in lasts before it is renewed or ends. |
| `auth.sessionMaxAge` | `24h` | How long a sign-in may be renewed for before the provider decides again. |
| `auth.sessionSecret` | `""` | Key that signs session cookies. Empty means sessions end with the pod. |
| `auth.existingSecret` | `""` | A secret you made yourself, instead of the chart's. |
| `auth.existingSecretKey` | `session-secret` | Key inside it. |
| `auth.proxy.userHeader` | `X-Forwarded-User` | |
| `auth.proxy.groupsHeader` | `X-Forwarded-Groups` | |
| `auth.proxy.secretHeader` | `X-Spinoza-Proxy-Secret` | Header carrying the proxy shared secret. |
| `auth.proxy.sharedSecret` | `""` | At least 32 bytes; authenticates the proxy. |
| `auth.proxy.existingSecret` | `""` | A secret holding the proxy shared secret. |
| `auth.proxy.existingSecretKey` | `proxy-secret` | Key inside it. |
| `auth.proxy.logoutURL` | `""` | Where signing out sends the browser. |
| `auth.proxy.webSocketMaxAge` | `5m` | Forces live connections to reconnect through the proxy; maximum `15m`. |
| `auth.oidc.issuerURL` | `""` | Your realm, as the browser reaches it. |
| `auth.oidc.internalIssuerURL` | `""` | The same provider on an address the pod can reach. |
| `auth.oidc.clientID` | `""` | |
| `auth.oidc.clientSecret` | `""` | Goes into a secret this chart creates. |
| `auth.oidc.existingSecret` | `""` | A secret you made yourself instead. |
| `auth.oidc.clientSecretKey` | `client-secret` | Key inside it. |
| `auth.oidc.redirectURL` | `publicURL` + `/auth/callback` | Register it with your provider. |
| `auth.oidc.postLogoutURL` | `publicURL` + `/` | Register it too. |
| `auth.oidc.scopes` | `openid profile email groups` | Drop `groups` for Google. |
| `auth.oidc.groupsClaim` | `groups` | |
| `auth.oidc.usernameClaims` | `preferred_username,email,sub` | First one present wins. |
| `auth.oidc.usernamePrefix` | `""` | Match what your apiserver binds. |
| `auth.oidc.groupsPrefix` | `""` | |
| `auth.oidc.caCert` | `""` | Path to a CA you mounted with `extraVolumes`. |
| `auth.oidc.insecureSkipVerify` | `false` | For a lab, never for real use. |
| `auth.oidc.unsafeAllowHTTP` | `false` | Allow plaintext non-loopback issuer endpoints. Permits network authentication compromise. |
| `auth.oidc.backchannelLogout` | `false` | Accept IdP-initiated session revocation. |
| `rbac.create` | `true` | |
| `rbac.read` | `workloads` | `workloads`, or the unsafe `everything` compatibility mode that includes Secrets. |
| `rbac.write` | `false` | Unsafe compatibility mode granting the pod write access when impersonation is off. |
| `rbac.impersonation.users` | `[]` | Exact usernames the pod may impersonate. |
| `rbac.impersonation.groups` | `[]` | Exact groups the pod may impersonate. |
| `rbac.impersonation.unsafeAllowAnyUser` | `false` | Permit impersonating any username. |
| `rbac.impersonation.unsafeAllowAnyGroup` | `false` | Permit impersonating any group, including privileged groups. |
| `rbac.extraRules` | `[]` | Extra ClusterRole rules. |
| `serviceAccount.create` | `true` | |
| `service.port` | `8080` | |
| `ingress.enabled` | `false` | |
| `httpRoute.enabled` | `false` | Gateway API instead of an Ingress. Needs `httpRoute.parentRefs`. |
| `httpRoute.parentRefs` | `[]` | The Gateway that carries the route. |
| `httpRoute.hostnames` | `[]` | |
| `metrics.enabled` | `true` | Spinoza's own metrics at `/metrics`. |
| `metrics.separatePort` | `false` | Also listen on `metrics.port` with no session, for a scrape. |
| `metrics.port` | `9090` | |
| `metrics.serviceMonitor.enabled` | `false` | Needs `metrics.separatePort`. |
| `metrics.serviceMonitor.interval` | `30s` | |
| `metrics.serviceMonitor.labels` | `{}` | What your Prometheus operator selects on. |
| `audit.interval` | `""` | Re-run the checks on a timer, as a duration such as `1h`. |
| `audit.webhookURL` | `""` | Posts a JSON summary when a run differs from the baseline. |
| `audit.retention` | `""` | How long recorded changes and audit runs are kept. Empty keeps them. |
| `networkPolicy.enabled` | `false` | Needs `networkPolicy.ingressFrom`. |
| `networkPolicy.ingressFrom` | `[]` | Peers allowed to reach spinoza, usually your ingress controller. |
| `networkPolicy.apiServerCIDR` | `""` | Narrow egress to your apiserver. Empty allows 443 and 6443 anywhere. |
| `networkPolicy.allowInternet` | `true` | Egress on 443, which a hosted provider and chart repositories need. |
| `networkPolicy.egressTo` | `[]` | Extra egress rules, rendered as given. |
| `podDisruptionBudget.enabled` | `false` | |
| `podDisruptionBudget.maxUnavailable` | `1` | |
| `terminationGracePeriodSeconds` | `45` | How long the pod gets to close views, log streams and terminals. |
| `persistence.enabled` | `false` | Keeps per-user settings, baselines and the timeline across restarts. |
| `persistence.existingClaim` | `""` | Use a claim you made yourself; the chart then renders none. |
| `extraObjects` | `[]` | Rendered through `tpl` after everything else. |
| `resources` | 100m / 256Mi requested, 1Gi limit | |
| `extraArgs`, `extraEnv`, `extraVolumes`, `extraVolumeMounts` | `[]` | |

`nameOverride`, `fullnameOverride`, `imagePullSecrets`, `podAnnotations`,
`podLabels`, `podSecurityContext`, `securityContext`, `nodeSelector`,
`tolerations`, `affinity`, `topologySpreadConstraints` and `priorityClassName`
behave the way they do in every other chart.

## What browsers it is built for

The workspace is built for **1280×720 and up**, checked in CI at 1280×720 and
1440×900 and at 200% browser zoom. Narrower than 1280 it scrolls sideways rather
than reflowing, and a phone is out of scope — there is no small-screen layout to
fall back to, so don't put spinoza on a URL people reach from one and expect it
to work.

A fresh profile opens the **Focused** preset: one workspace, side and bottom
docks closed. **Console** opens every dock and wants the width to match.
Settings → Panels switches between them, and a layout somebody arranged
themselves is never overwritten.

## What the chart refuses to render

- No `publicURL`.
- No explicit `auth.mode`, or a value other than `none`, `proxy` or `oidc`.
- `auth.mode: none` without `auth.allowAnonymous: true`.
- `auth.mode: proxy` without a proxy shared secret of at least 32 bytes.
- `auth.mode: oidc` with no `issuerURL` or no `clientID`.
- A plaintext `publicURL` without `unsafeAllowHTTP: true`.
- A plaintext OIDC issuer without `auth.oidc.unsafeAllowHTTP: true`.
- `impersonate: true` without an exact username list or the explicit
  `rbac.impersonation.unsafeAllowAnyUser` compatibility mode.
- `networkPolicy.enabled` with an empty `ingressFrom`. A policy that selects the
  pod and names no peer denies everything to it, so an empty list would take
  spinoza off the network rather than protect it.
- `podDisruptionBudget` with both `minAvailable` and `maxUnavailable`, or with
  neither.
- `podDisruptionBudget.minAvailable`. Spinoza runs as one replica, so a budget
  that keeps one pod available refuses every voluntary eviction and a node drain
  waits forever. `maxUnavailable: 1` is the one that makes sense here: it lets
  the node drain, and the browser reconnects when the pod comes back.
- `metrics.serviceMonitor.enabled` without `metrics.separatePort`. On the main
  port `/metrics` answers admins only, and a scrape carries no session.
- `ingress.enabled` and `httpRoute.enabled` together, or `httpRoute.enabled`
  with no `parentRefs`. Two front doors for one `publicURL` means spinoza
  refuses whichever request arrives with the other origin.

## What a NetworkPolicy will break

The rendered policy lets in only the peers you name and lets out DNS, the
apiserver, and — while `allowInternet` is on — anything on 443. If your identity
provider, your chart repositories or your Prometheus sit somewhere else, name
them in `networkPolicy.egressTo` before you turn the policy on. A sign-in that
hangs at the provider and a Helm view with no charts are both what this looks
like when a rule is missing.

`metrics.separatePort` opens an unauthenticated port. Keep it inside the
cluster: reach it with a ServiceMonitor or a scrape annotation, and never
through the ingress.

## Security default migration

The chart now installs a read-only workload viewer. Its service account cannot
read Secrets, impersonate another principal, or write cluster objects. Existing
installations that intentionally relied on the former broad defaults must set
`rbac.read: everything` explicitly. To keep impersonation, set
`impersonate: true`, list every permitted username and group under
`rbac.impersonation`, or consciously select the `unsafeAllowAnyUser` and
`unsafeAllowAnyGroup` compatibility settings. The chart never grants
service-account impersonation.
