{{/* Expand the name of the chart. */}}
{{- define "upo.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Create a default fully qualified app name. */}}
{{- define "upo.fullname" -}}
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

{{- define "upo.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "upo.labels" -}}
helm.sh/chart: {{ include "upo.chart" . }}
{{ include "upo.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: unified-platform-operator
{{- end -}}

{{- define "upo.selectorLabels" -}}
app.kubernetes.io/name: {{ include "upo.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
control-plane: controller-manager
{{- end -}}

{{- define "upo.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "upo.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/* Stable names for webhook plumbing, referenced by several templates. */}}
{{- define "upo.webhookServiceName" -}}
{{- printf "%s-webhook" (include "upo.fullname" .) -}}
{{- end -}}

{{- define "upo.certificateName" -}}
{{- printf "%s-serving-cert" (include "upo.fullname" .) -}}
{{- end -}}

{{- define "upo.certificateSecretName" -}}
{{- printf "%s-webhook-cert" (include "upo.fullname" .) -}}
{{- end -}}

{{- define "upo.image" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion -}}
{{- printf "%s:%s" .Values.image.repository $tag -}}
{{- end -}}
