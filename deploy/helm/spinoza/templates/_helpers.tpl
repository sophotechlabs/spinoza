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

{{- define "spinoza.ownSecretNeeded" -}}
{{- if and (not .Values.auth.existingSecret) .Values.auth.sessionSecret -}}yes{{- end -}}
{{- if and (not .Values.auth.oidc.existingSecret) .Values.auth.oidc.clientSecret -}}yes{{- end -}}
{{- end -}}

{{- define "spinoza.validate" -}}
{{- if not .Values.publicURL -}}
{{- fail "spinoza needs publicURL, the address browsers reach it at" -}}
{{- end -}}
{{- if not (has .Values.auth.mode (list "none" "proxy" "oidc")) -}}
{{- fail (printf "auth.mode %q is not one of none, proxy, oidc" .Values.auth.mode) -}}
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
{{- end -}}
{{- if and (not .Values.impersonate) (not .Values.rbac.write) -}}
{{- fail "with impersonate off, spinoza acts as its own service account, so rbac.write has to be on for anything to change" -}}
{{- end -}}
{{- if gt (int .Values.replicaCount) 1 -}}
{{- fail "spinoza keeps back-channel logouts, the timeline and running port-forwards in the process, so a second replica would answer without them; replicaCount has to be 1" -}}
{{- end -}}
{{- end -}}
