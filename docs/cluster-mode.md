# Cluster mode

Spinoza normally runs on your own machine: loopback only, one token per run, your
kubeconfig, and it exits when the last window closes. Cluster mode is the other
shape. Spinoza runs as a pod, serves one cluster to a team over an ingress, and
asks people to sign in through your identity provider.

Turn it on with `--cluster-mode` and `--public-url`. The published Helm chart at
`oci://ghcr.io/sophotechlabs/charts/spinoza` sets both. Its source is in
[`deploy/helm/spinoza`](../deploy/helm/spinoza).

## What changes

| | your own machine | cluster mode |
|---|---|---|
| listens on | loopback only | whatever `--addr` says, `0.0.0.0:8080` by default |
| who gets in | the token printed at startup | whoever your identity provider says |
| what it acts as | your kubeconfig | a restricted pod service account by default; optionally the signed-in user through scoped impersonation |
| when it stops | when the last view closes | when the pod does |
| kubeconfigs, context switching, several clusters at once | yes | no; there is one cluster, the one it runs in |
| port-forwarding | yes | no; the forward would land on the server, not on you |
| a shell on the machine spinoza runs on | desktop app only | off |
| update check and self-install | yes | off |
| desktop window | yes | off |

Everything else is the same product: the same tables, the same GitOps and Helm
views, the same inspect drawer and the same exec. Spinoza's own port-forwarding
actions stay unavailable in cluster mode.

## Quick start

```sh
helm upgrade --install spinoza oci://ghcr.io/sophotechlabs/charts/spinoza \
  --namespace spinoza --create-namespace \
  --set publicURL=https://spinoza.example.com \
  --set auth.mode=oidc \
  --set auth.oidc.issuerURL=https://keycloak.example.com/realms/main \
  --set auth.oidc.clientID=spinoza \
  --set auth.oidc.clientSecret=... \
  --set auth.adminGroups='{platform-admins}' \
  --set ingress.enabled=true \
  --set ingress.className=nginx \
  --set 'ingress.hosts[0].host=spinoza.example.com' \
  --set 'ingress.hosts[0].paths[0].path=/'
```

Without an ingress, reach it with a port-forward:

```sh
kubectl -n spinoza port-forward svc/spinoza 8080:8080
```

`publicURL` is the HTTPS origin browsers use, such as
`https://spinoza.example.com`. It may end in `/`, but cannot contain a path,
query, fragment, or credentials. Spinoza refuses a request whose `Origin` is
some other site, marks the session cookie `Secure` when it is https, and derives
the OIDC callback from it. A request with no `Origin` — a kubelet probe, `curl`,
the page's own navigation — is not one a browser made across sites, so it goes
through. Plaintext HTTP is accepted only for loopback development addresses or
with the explicit `unsafeAllowHTTP` compatibility setting, which exposes
sessions, WebSockets, and cluster data to network interception.

## Signing in

`auth.mode` picks how spinoza learns who you are.

### `oidc`

Spinoza runs the login itself: authorization code with PKCE, a nonce, the ID
token verified against the provider's JWKS, and a session cookie signed with
HMAC-SHA256. Nothing about the session is stored server-side.

Keycloak, as an example. Create a client in your realm:

- Client ID: `spinoza`
- Client authentication: on (this is a confidential client)
- Valid redirect URI: `https://spinoza.example.com/auth/callback`
- Valid post logout redirect URI: `https://spinoza.example.com/`
- Web origins: `https://spinoza.example.com`

Then add a **Group Membership** mapper on the client, token claim name `groups`,
"Full group path" off, and tick "Add to ID token". Spinoza reads groups from the
ID token, not from userinfo.

```yaml
publicURL: https://spinoza.example.com
auth:
  mode: oidc
  oidc:
    issuerURL: https://keycloak.example.com/realms/main
    clientID: spinoza
    existingSecret: spinoza-oidc
    clientSecretKey: client-secret
  adminGroups: [platform-admins]
  editorGroups: [platform, sre]
```

Other providers work the same way. Spinoza asks for `openid profile email groups`
but drops any scope the provider's discovery document does not list, so stock
Keycloak, which defines no `groups` scope, gets `openid profile email` and Dex,
which needs `groups` before it will emit the claim, gets all four. The startup
log says what was left out. `auth.oidc.scopes` overrides the list outright.

**Which claim becomes the username.** Spinoza takes the first of
`preferred_username`, `email`, `sub` that the token carries, and that string is
what it impersonates and what your RBAC has to bind. Change the order with
`auth.oidc.usernameClaims`. If your apiserver is configured with
`--oidc-username-prefix`, set the same prefix here with
`auth.oidc.usernamePrefix`, and likewise for groups.

**The provider on two addresses.** When the browser reaches Keycloak at its
public URL and the pod reaches it through service DNS, set `issuerURL` to the
public one and `internalIssuerURL` to the internal one. Spinoza fetches
discovery internally, keeps validating the token's `iss` against the public
issuer, and sends the browser to the public authorize and logout endpoints.
Both addresses must use HTTPS outside loopback. Use `caCert` with an internal
certificate authority. The separate `auth.oidc.unsafeAllowHTTP` setting exists
for disposable labs and permits a network attacker to compromise authentication
and observe the OIDC client secret.

The RBAC index — who may do what across the cluster — is admin-only here.
It is served from the shared cache, so it would otherwise show every binding to
anyone who can read the cluster, which the built-in `view` role does not grant.

`--pprof` still works, and the profiles it serves answer to admins only: a heap
dump carries whatever the caches hold, which under the default read role
excludes Secrets. The process-argument profile is not served because arguments
can contain credentials supplied through command-line flags.

**Signing out.** Spinoza clears its own cookie and, when the provider advertises
`end_session_endpoint`, sends the browser there so the SSO session ends too.
Without one, the next login carries `prompt=login` so the provider shows its
form instead of silently signing the same person back in.

Spinoza sends `client_id` and `post_logout_redirect_uri`, not `id_token_hint`,
which keeps a bearer token out of the browser's cookie jar. Keycloak asks "do
you want to log out?" before it goes ahead, which is what the spec says a
provider should do without the hint.

**Back-channel logout.** With `auth.oidc.backchannelLogout: true`, register
`https://spinoza.example.com/auth/backchannel-logout` at the provider. Disabling
somebody there then ends their spinoza session at once instead of at cookie
expiry. It needs a `sid` in the logout token, which Keycloak, Okta, Auth0 and
Entra all send. Revocations live in memory, so they are lost on restart and are
not shared between replicas.

### `proxy`

Use this when oauth2-proxy, Pomerium, Authelia or similar already fronts your
internal tools. Spinoza reads `X-Forwarded-User` and `X-Forwarded-Groups` and
trusts them only when the request also carries the configured proxy secret.

```yaml
auth:
  mode: proxy
  proxy:
    existingSecret: spinoza-proxy
    existingSecretKey: proxy-secret
    logoutURL: https://spinoza.example.com/oauth2/sign_out
```

Configure the proxy to set `X-Spinoza-Proxy-Secret` from the same Kubernetes
Secret. It must still strip the identity and secret headers from incoming
requests. Direct requests cannot forge an identity without knowing this secret.
The original upgrade headers remain inside an open WebSocket, so Spinoza closes
proxy-authenticated live sessions after `auth.proxy.webSocketMaxAge`, five
minutes by default and at most fifteen minutes. The browser reconnects through
the proxy with current identity headers.

The Cluster Mode gate tests this with oauth2-proxy `v7.15.3`. Its public ingress
points to oauth2-proxy, while spinoza remains reachable only as a ClusterIP.
The proxy's structured configuration sends the authenticated claims and the
secret to that upstream:

```yaml
upstreamConfig:
  upstreams:
    - id: spinoza
      path: /
      uri: http://spinoza.spinoza.svc.cluster.local:8080
      passHostHeader: true
      proxyWebSockets: true
providers:
  - id: identity
    provider: oidc
    clientID: spinoza-proxy
    clientSecretFile: /var/run/secrets/oauth2-proxy/client-secret
    backendLogoutURL: "https://identity.example.com/realms/main/protocol/openid-connect/logout?id_token_hint={id_token}"
    oidcConfig:
      issuerURL: https://identity.example.com/realms/main
      groupsClaim: groups
      userIDClaim: preferred_username
injectRequestHeaders:
  - name: X-Forwarded-User
    values:
      - claimSource:
          claim: preferred_username
  - name: X-Forwarded-Groups
    values:
      - claimSource:
          claim: groups
  - name: X-Spinoza-Proxy-Secret
    values:
      - secretSource:
          fromFile: /var/run/secrets/oauth2-proxy/proxy-secret
```

Use `--alpha-config` for that file and set the normal cookie, redirect URL,
email-domain and reverse-proxy options separately. oauth2-proxy strips an
incoming value for each injected header by default and normalizes header names,
so alternate capitalization or underscores cannot retain a forged identity.
Keep `preserveRequestValue` off. The structured format is marked alpha and may
change between oauth2-proxy releases; validate it with `--config-test` before an
upgrade. See oauth2-proxy's
[header injection reference](https://oauth2-proxy.github.io/oauth2-proxy/configuration/alpha-config/)
for the version-specific schema.

`/oauth2/sign_out` clears only oauth2-proxy's cookie. Set the provider's
`backendLogoutURL` with `{id_token}` as above, or configure an equivalent `rd`
redirect to the provider's logout endpoint, so its SSO session ends too. Without
that, returning to the protected root can silently sign the same person in
again.

### `none`

No sign-in at all: everybody who reaches the address is an admin here. The chart
does not select this mode by default and requires `auth.allowAnonymous: true`
when you explicitly choose it.

## Sessions

The cookie holds the username, the groups, the role and the provider's session
id, signed with `auth.sessionSecret`. It is renewed once it is past half its
life, so somebody working all day is never bounced back to the provider in the
middle of a request — but only up to `auth.sessionMaxAge`, 24 hours by default.

That cap matters more than it looks. The groups in the cookie are the groups
spinoza impersonates, so without a cap a browser left open would keep acting on
somebody's old group membership for as long as they kept clicking. Past the cap
the cookie runs out, the browser goes back to the provider, and the provider
decides again. Back-channel logout closes the gap in between when your provider
supports it. Leave the secret empty and one is
generated in the pod, which means every sign-in ends when the pod restarts. For
anything long-lived, set it, or point `auth.existingSecret` at a secret you made
yourself.

A very large group list will not fit in a cookie. Spinoza says so at sign-in
rather than issuing a session that browsers silently drop.

## What each person may do

Two things decide that, and both have to allow it.

**A role in spinoza**, from the groups the provider sent:

| role | may |
|---|---|
| `viewer` | read permitted views, save personal settings, and change no cluster objects |
| `editor` | everything above, plus apply, delete, scale, restart, reconcile, and Helm install, upgrade, rollback and uninstall |
| `admin` | everything above, plus exec, debug containers, node shells, cordon, drain, the RBAC index, the profiler and protecting a cluster |

```yaml
auth:
  defaultRole: viewer
  adminGroups: [platform-admins]
  editorGroups: [platform, sre]
  viewerGroups: [everyone]
```

The strongest matching list wins. With no lists at all, everybody gets
`defaultRole`.

Appearance, layout, table, check-rule and other browser settings belong to the
signed-in user. They are stored under an identity digest, are never returned to
another user, and do not require an elevated role. Cluster-wide mute decisions,
timeline retention and other deployment state are kept outside that personal
settings endpoint.

**Kubernetes RBAC**, through optional impersonation. With `impersonate: true`,
every apiserver call spinoza makes for a request carries
`Impersonate-User` and `Impersonate-Group`, so the cluster answers as if the
person had asked it themselves. A write they have no binding for comes back 403
from the apiserver, not from spinoza. The buttons follow the same answer: the
per-object capability checks are `SelfSubjectAccessReview`s made under
impersonation, so what a person cannot do is greyed out with the reason.

Verify what somebody has:

```sh
kubectl auth can-i --list --as=alice@example.com --as-group=platform
```

Impersonation is off by default. When it is enabled, the chart requires exact
usernames in `rbac.impersonation.users`. List permitted groups in
`rbac.impersonation.groups` too. Large deployments may explicitly select
`unsafeAllowAnyUser` or `unsafeAllowAnyGroup` for compatibility, but the latter
allows a claimed group such as `system:masters` and should only be used when the
identity proxy or provider enforces that boundary. The chart never grants
permission to impersonate service accounts.

```yaml
impersonate: true
rbac:
  impersonation:
    users: [alice@example.com, bob@example.com]
    groups: [platform, sre]
```

## What each person may see

Reads are the one place where spinoza and radar-style dashboards have to make a
choice, because tables are served from shared informer caches that were filled
with the service account's own rights, not the reader's.

When impersonation is enabled, Spinoza resolves it this way:

- **Namespaces.** On the first read of a request, spinoza asks the cluster which
  namespaces the signed-in user may read. Every shared table feed then checks
  exact `list` and `watch` access for its API group, resource, and namespace.
  Search, metrics, counts, and custom checks use the same identity boundary.
- **Kinds that belong to no namespace** — nodes, persistent volumes, storage
  classes, cluster-scoped custom resources — are not shown to an account that
  reads named namespaces only. Subscribing to one comes back refused rather than
  empty.
- **Views that read the whole cluster** — the overview, checks, the issue queue,
  topology, the GitOps and Flux and Argo dashboards, traffic, metrics, compare,
  the RBAC index and the fleet reports — need cluster-wide read. An account without it gets a
  403 that says so, rather than a partial answer that looks complete.
- **Everything read straight from the apiserver** — a single object's YAML,
  events, logs, exec and Helm release storage — is impersonated, so the cluster
  decides.

The practical shape: give people cluster-wide read and let RBAC govern writes,
or bind them in their own namespaces and accept that the cluster-wide views are
closed to them. If you need a harder boundary than that, run one spinoza per
trust boundary with a namespace-scoped service account.

An answer the cluster refuses to give — a webhook authorizer that times out, say
— counts as no, not as yes. It is not silently dropped either: the account menu
names how many namespaces the cluster would not decide about, so a narrower list
than you expect reads as an unanswered question rather than as a refusal.

A live feed, log stream, exec, or node shell rechecks its admitted Kubernetes
permissions on a short interval. A denial, authorizer error, or timeout closes
the session. Everything a person changes is also checked by the apiserver at
the moment they do it.

## The service account's own rights

`rbac.read` picks what the pod itself may cache.

- `workloads` (default): the common groups without Secrets. Custom resources then need
  `rbac.extraRules`, and the Helm view stays empty.
- `everything`: the explicit unsafe compatibility mode with `get`, `list` and
  `watch` on every group and resource. It includes Secrets, which is where Helm
  keeps its release history.

With `impersonate: true` the service account needs no write rights at all —
changes ride the signed-in user's own bindings. `rbac.write: true` is an unsafe
compatibility setting for `impersonate: false`, where changes run as the pod.

## Helm and kubectl inside the pod

Upgrades, rollbacks, uninstalls and debug containers shell out to `helm` and
`kubectl`, and both act as the signed-in user through `--kube-as-user` and
`--as`. Passing those flags stops either tool falling back to the pod's own
service-account credentials, so spinoza writes a kubeconfig for them at
`$XDG_CONFIG_HOME/spinoza/in-cluster.kubeconfig` naming the mounted token and CA
file. It needs somewhere writable, which the chart gives it; without one, both
tools act as spinoza itself and the startup log says so.

## Storage

Settings, audit baselines, mutes and the timeline live under `/var/lib/spinoza`.
An admin chooses workload or wide timeline recording from the History view. The
choice and recorded changes resume after a pod replacement when
`persistence.enabled` is on. Otherwise the state volume is an `emptyDir` and
they go with the pod.

## Running several replicas

Don't, for now. Sessions are stateless cookies and would survive a load
balancer, but back-channel revocations are per-pod, the timeline recorder writes
to one state database, and live subscriptions and terminal sessions belong to
the process that accepted them. `replicaCount: 1` is the supported shape, and
the chart refuses to render anything else rather than install something that
half works.

## Troubleshooting

**Everybody lands back on the sign-in page.** Check the pod logs for
`a login did not complete`. The message is the one the sign-in page shows, and
it names what the provider actually said.

**`the id token did not verify`.** Usually the audience: the token's `aud` has
to be the client id spinoza was given.

**`the id token carries none of the claims spinoza reads a username from`.**
The provider is not sending `preferred_username`, `email` or `sub` in the ID
token. Add a mapper, or point `auth.oidc.usernameClaims` at what it does send.

**Signed in, but every table is empty.** The account has no namespace it may
list pods in. `kubectl auth can-i list pods --all-namespaces --as=<user>` says
so, and a `RoleBinding` fixes it.

**Every write comes back 403.** That is the apiserver, not spinoza, unless the
message starts with `your role here is` — that one is spinoza's role gate, and
the fix is a group list in `auth`.

**`spinoza answers pages served from its own address only`.** The request's
`Origin` was not `publicURL`. Usually `publicURL` names `https` while the browser
reached it over `http`, or the other way round.
