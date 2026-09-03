{{- define "spinoza.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "spinoza.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "spinoza.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "spinoza.labels" -}}
helm.sh/chart: {{ include "spinoza.chart" . }}
{{ include "spinoza.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "spinoza.selectorLabels" -}}
app.kubernetes.io/name: {{ include "spinoza.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "spinoza.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "spinoza.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "spinoza.sessionSecretName" -}}
{{- default (printf "%s-auth" (include "spinoza.fullname" .)) .Values.auth.existingSecret -}}
{{- end -}}

{{- define "spinoza.clientSecretName" -}}
{{- default (printf "%s-auth" (include "spinoza.fullname" .)) .Values.auth.oidc.existingSecret -}}
{{- end -}}

{{- define "spinoza.proxySecretName" -}}
{{- default (printf "%s-auth" (include "spinoza.fullname" .)) .Values.auth.proxy.existingSecret -}}
{{- end -}}

{{- define "spinoza.ownSecretNeeded" -}}
{{- if and (not .Values.auth.existingSecret) .Values.auth.sessionSecret -}}yes{{- end -}}
{{- if and (not .Values.auth.oidc.existingSecret) .Values.auth.oidc.clientSecret -}}yes{{- end -}}
{{- if and (eq .Values.auth.mode "proxy") (not .Values.auth.proxy.existingSecret) .Values.auth.proxy.sharedSecret -}}yes{{- end -}}
{{- end -}}

{{- define "spinoza.validate" -}}
{{- if not .Values.publicURL -}}
{{- fail "spinoza needs publicURL, the address browsers reach it at" -}}
{{- end -}}
{{- if and (hasPrefix "http://" .Values.publicURL) (not .Values.unsafeAllowHTTP) -}}
{{- fail "publicURL must use https; plaintext http requires unsafeAllowHTTP=true" -}}
{{- end -}}
{{- if not .Values.auth.mode -}}
{{- fail "auth.mode must be set explicitly to oidc, proxy or none" -}}
{{- end -}}
{{- if not (has .Values.auth.mode (list "none" "proxy" "oidc")) -}}
{{- fail (printf "auth.mode %q is not one of none, proxy, oidc" .Values.auth.mode) -}}
{{- end -}}
{{- if eq .Values.auth.mode "none" -}}
{{- if not .Values.auth.allowAnonymous -}}
{{- fail "auth.mode none requires auth.allowAnonymous=true because every caller becomes an admin" -}}
{{- end -}}
{{- end -}}
{{- if eq .Values.auth.mode "proxy" -}}
{{- if not (or .Values.auth.proxy.existingSecret .Values.auth.proxy.sharedSecret) -}}
{{- fail "auth.mode proxy requires auth.proxy.sharedSecret or auth.proxy.existingSecret" -}}
{{- end -}}
{{- if and .Values.auth.proxy.sharedSecret (lt (len .Values.auth.proxy.sharedSecret) 32) -}}
{{- fail "auth.proxy.sharedSecret must be at least 32 bytes" -}}
{{- end -}}
{{- end -}}
{{- if not (has .Values.rbac.read (list "everything" "workloads")) -}}
{{- fail (printf "rbac.read %q is not one of everything, workloads" .Values.rbac.read) -}}
{{- end -}}
{{- if eq .Values.auth.mode "oidc" -}}
{{- if not .Values.auth.oidc.issuerURL -}}
{{- fail "auth.oidc.issuerURL is required in oidc mode" -}}
{{- end -}}
{{- if not .Values.auth.oidc.clientID -}}
{{- fail "auth.oidc.clientID is required in oidc mode" -}}
{{- end -}}
{{- if and (hasPrefix "http://" .Values.auth.oidc.issuerURL) (not .Values.auth.oidc.unsafeAllowHTTP) -}}
{{- fail "auth.oidc.issuerURL must use https; plaintext http requires auth.oidc.unsafeAllowHTTP=true" -}}
{{- end -}}
{{- if and (hasPrefix "http://" .Values.auth.oidc.internalIssuerURL) (not .Values.auth.oidc.unsafeAllowHTTP) -}}
{{- fail "auth.oidc.internalIssuerURL must use https; plaintext http requires auth.oidc.unsafeAllowHTTP=true" -}}
{{- end -}}
{{- end -}}
{{- if and .Values.impersonate (not .Values.rbac.impersonation.unsafeAllowAnyUser) (empty .Values.rbac.impersonation.users) -}}
{{- fail "impersonate=true requires rbac.impersonation.users or the explicit unsafe rbac.impersonation.unsafeAllowAnyUser=true compatibility mode" -}}
{{- end -}}
{{- if gt (int .Values.replicaCount) 1 -}}
{{- fail "spinoza keeps back-channel logouts, the timeline and running port-forwards in the process, so a second replica would answer without them; replicaCount has to be 1" -}}
{{- end -}}
{{- end -}}
