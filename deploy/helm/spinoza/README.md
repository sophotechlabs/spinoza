# spinoza

Serves one Kubernetes cluster to a team from inside it: spinoza as a pod, behind
your ingress, with your identity provider deciding who gets in.

The guide is [docs/cluster-mode.md](../../../docs/cluster-mode.md). This file is
the values reference.

```sh
helm upgrade --install spinoza deploy/helm/spinoza \
  --namespace spinoza --create-namespace \
  --set publicURL=https://spinoza.example.com
```

`publicURL` is required. The chart refuses to render without it.

## Values

| key | default | what it does |
|---|---|---|
| `publicURL` | — | The address browsers reach spinoza at. Required. |
| `replicaCount` | `1` | Has to be one; anything else is refused. See the guide. |
| `image.repository` | `ghcr.io/sophotechlabs/spinoza` | |
| `image.tag` | chart `appVersion` | |
| `impersonate` | `true` | Act on the cluster as the signed-in user. |
| `logLevel` | `info` | `debug`, `info`, `warn` or `error`. |
| `prometheus` | `""` | `namespace/service:port` for metric history; discovered when empty. |
| `nodeShell` | `false` | Allow a root shell on a node, which creates a privileged pod. |
| `auth.mode` | `none` | `none`, `proxy` or `oidc`. |
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
| `auth.proxy.logoutURL` | `""` | Where signing out sends the browser. |
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
| `auth.oidc.backchannelLogout` | `false` | Accept IdP-initiated session revocation. |
| `rbac.create` | `true` | |
| `rbac.read` | `everything` | `everything` or `workloads`. |
| `rbac.write` | `false` | Only needed with `impersonate: false`. |
| `rbac.extraRules` | `[]` | Extra ClusterRole rules. |
| `serviceAccount.create` | `true` | |
| `service.port` | `8080` | |
| `ingress.enabled` | `false` | |
| `persistence.enabled` | `false` | Keeps settings, baselines and the timeline across restarts. |
| `resources` | 100m / 256Mi requested, 1Gi limit | |
| `extraArgs`, `extraEnv`, `extraVolumes`, `extraVolumeMounts` | `[]` | |

`nameOverride`, `fullnameOverride`, `imagePullSecrets`, `podAnnotations`,
`podLabels`, `podSecurityContext`, `securityContext`, `nodeSelector`,
`tolerations`, `affinity`, `topologySpreadConstraints` and `priorityClassName`
behave the way they do in every other chart.

## What the chart refuses to render

- No `publicURL`.
- An `auth.mode` that is not `none`, `proxy` or `oidc`.
- `auth.mode: oidc` with no `issuerURL` or no `clientID`.
- `impersonate: false` together with `rbac.write: false`, which would leave
  spinoza unable to change anything at all.
