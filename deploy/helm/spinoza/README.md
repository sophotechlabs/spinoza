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
| `persistence.enabled` | `false` | Keeps per-user settings, baselines and the timeline across restarts. |
| `resources` | 100m / 256Mi requested, 1Gi limit | |
| `extraArgs`, `extraEnv`, `extraVolumes`, `extraVolumeMounts` | `[]` | |

`nameOverride`, `fullnameOverride`, `imagePullSecrets`, `podAnnotations`,
`podLabels`, `podSecurityContext`, `securityContext`, `nodeSelector`,
`tolerations`, `affinity`, `topologySpreadConstraints` and `priorityClassName`
behave the way they do in every other chart.

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

## Security default migration

The chart now installs a read-only workload viewer. Its service account cannot
read Secrets, impersonate another principal, or write cluster objects. Existing
installations that intentionally relied on the former broad defaults must set
`rbac.read: everything` explicitly. To keep impersonation, set
`impersonate: true`, list every permitted username and group under
`rbac.impersonation`, or consciously select the `unsafeAllowAnyUser` and
`unsafeAllowAnyGroup` compatibility settings. The chart never grants
service-account impersonation.
