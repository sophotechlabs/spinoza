# Modes, accounts and what a feature is waiting for

Spinoza runs four ways. The screens are the same; who acts on the cluster and what is
available are not. When a view is greyed out or says it is partial, the answer is almost
always one of the prerequisites below.

## Who acts on the cluster

| Mode | How it starts | The account the cluster sees | Where the state lives |
|---|---|---|---|
| Local browser | `spinoza`, loopback only, a token minted per run | your kubeconfig user, credential plugins included | `~/.config/spinoza` on your machine |
| Desktop | the packaged app, same server inside a window | your kubeconfig user, plus a local shell and file picker | the same directory |
| Team, cluster mode | the container behind your ingress, `--cluster-mode` | the chart's service account, or the signed-in person when `impersonate: true` | the pod's volume, kept across restarts with `persistence.enabled` |
| MCP | `spinoza-mcp`, one context over stdio | your kubeconfig user | none; every call reads the cluster |

Cluster mode adds a second gate in front of the cluster's own RBAC: viewer, editor and admin
roles from your identity provider's groups. Editor is what spinoza will let you ask for; the
cluster still decides whether the service account, or you when impersonated, may do it.

## What each feature needs

| Feature | Needs | Without it |
|---|---|---|
| Resource tables | list and watch on the kind | a namespace-only kubeconfig or role browses one namespace at a time; "All namespaces" says to pick one |
| Secrets | read access to Secrets; in cluster mode `rbac.read: everything` | the kind is missing from the tree |
| Custom resources in cluster mode | `rbac.extraRules` naming them | the kinds are missing, and the Helm view stays empty |
| Metrics charts | `metrics.k8s.io` for the person asking, and Prometheus for ranges over an hour | a reader without metrics access is refused; without Prometheus, spinoza samples every 15 seconds while the panel is open and labels the series as sampled by spinoza |
| Traffic | Cilium Hubble flow metrics scraped by Prometheus | the view says no traffic source was found |
| Flux and Argo views | the controllers installed in the cluster | the sidebar says the controller is not found |
| Helm | the release secrets or configmaps readable | releases are missing; actions also need the `helm` binary, which the image ships |
| Node shell | `nodeShell: true`, the admin role, and permission to create the privileged pod | the action is absent |
| Terminal transcripts | `recordSessions: true` | sessions run but nothing is kept |
| Scheduled audits and the webhook | `audit.interval` and `audit.webhookURL` | checks run only when opened |
| Update check | local and desktop modes only | cluster mode never calls out for it |

## How a missing prerequisite shows

- **Loading** while the first answer is on its way.
- **Empty** with a sentence saying what was looked for and not found.
- **Partial data** across the top when one source answered and another did not; the
  sentence names the source, and the counts below it are what could be counted.
- **Stale** when the last answer is being shown because a newer one failed; retry is offered.
- **Failed** with the cluster's own reason, including RBAC refusals that name the account.

The support bundle at `/api/support`, admins only in cluster mode, carries the versions, the
readiness of the catalog, caches and store, and which capabilities failed, with secret-looking
values redacted. Attach it when asking for help.

## The next step, by symptom

| You see | Do |
|---|---|
| a table that says your account reads named namespaces only | pick a namespace; ask for a ClusterRoleBinding only if you need every namespace |
| a 403 on metrics history for a colleague but not for you | grant `list` on `pods.metrics.k8s.io` in that namespace, or accept the refusal |
| "sampled by spinoza" on the metrics panel | point `prometheus` at your Prometheus service to get history the cluster kept |
| Flux or Argo missing from the sidebar | install the controller; spinoza reads its objects, it does not replace it |
| Helm empty in cluster mode | set `rbac.read: everything` or add the release secrets to `rbac.extraRules` |
| a delete refused because the object was replaced | inspect it again; the name now belongs to a different object |
| a write that timed out | check the object before trying again; the change may already be on the cluster and its audit row will say |
