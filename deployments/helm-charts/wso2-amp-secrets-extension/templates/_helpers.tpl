{{/*
Expand the name of the chart.
*/}}
{{- define "wso2-amp-secrets-extension.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "wso2-amp-secrets-extension.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "wso2-amp-secrets-extension.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "wso2-amp-secrets-extension.labels" -}}
helm.sh/chart: {{ include "wso2-amp-secrets-extension.chart" . }}
{{ include "wso2-amp-secrets-extension.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: wso2-amp
{{- end }}

{{/*
Selector labels
*/}}
{{- define "wso2-amp-secrets-extension.selectorLabels" -}}
app.kubernetes.io/name: {{ include "wso2-amp-secrets-extension.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
