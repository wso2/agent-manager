{{/*
Expand the name of the chart.
*/}}
{{- define "agent-management-platform.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
Uses simple naming: "amp" instead of release-name-chart-name format
*/}}
{{- define "agent-management-platform.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- print "amp" | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "agent-management-platform.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "agent-management-platform.labels" -}}
helm.sh/chart: {{ include "agent-management-platform.chart" . }}
{{ include "agent-management-platform.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "agent-management-platform.selectorLabels" -}}
app.kubernetes.io/name: {{ include "agent-management-platform.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
Uses simple naming: "amp" instead of release-name-chart-name
*/}}
{{- define "agent-management-platform.serviceAccountName" -}}
{{- if or .Values.serviceAccount.create .Values.rbac.create }}
{{- default "amp" .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
==============================================
Agent Manager Service Helpers
==============================================
*/}}

{{/*
Agent Manager Service fullname
Uses simple naming: "amp-api" instead of release-name-chart-name-agent-manager-service
*/}}
{{- define "agent-management-platform.agentManagerService.fullname" -}}
{{- print "amp-api" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Agent Manager Service labels
*/}}
{{- define "agent-management-platform.agentManagerService.labels" -}}
{{ include "agent-management-platform.labels" . }}
app.kubernetes.io/component: agent-manager-service
{{- end }}

{{/*
Agent Manager Service selector labels
*/}}
{{- define "agent-management-platform.agentManagerService.selectorLabels" -}}
{{ include "agent-management-platform.selectorLabels" . }}
app.kubernetes.io/component: agent-manager-service
{{- end }}

{{/*
==============================================
Console Helpers
==============================================
*/}}

{{/*
Console fullname
Uses simple naming: "amp-console" instead of release-name-chart-name-console
*/}}
{{- define "agent-management-platform.console.fullname" -}}
{{- print "amp-console" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Console labels
*/}}
{{- define "agent-management-platform.console.labels" -}}
{{ include "agent-management-platform.labels" . }}
app.kubernetes.io/component: console
{{- end }}

{{/*
Console selector labels
*/}}
{{- define "agent-management-platform.console.selectorLabels" -}}
{{ include "agent-management-platform.selectorLabels" . }}
app.kubernetes.io/component: console
{{- end }}

{{/*
==============================================
Database Helpers
==============================================
*/}}

{{/*
PostgreSQL host
Uses simple naming: "amp-postgresql" instead of release-name-postgresql
*/}}
{{- define "agent-management-platform.postgresql.host" -}}
{{- if .Values.postgresql.enabled }}
{{- print "amp-postgresql" }}
{{- else }}
{{- .Values.postgresql.external.host }}
{{- end }}
{{- end }}

{{/*
PostgreSQL port
*/}}
{{- define "agent-management-platform.postgresql.port" -}}
{{- if .Values.postgresql.enabled }}
{{- print "5432" }}
{{- else }}
{{- .Values.postgresql.external.port }}
{{- end }}
{{- end }}

{{/*
PostgreSQL database name
*/}}
{{- define "agent-management-platform.postgresql.database" -}}
{{- if .Values.postgresql.enabled }}
{{- .Values.postgresql.auth.database }}
{{- else }}
{{- .Values.postgresql.external.database }}
{{- end }}
{{- end }}

{{/*
PostgreSQL username
*/}}
{{- define "agent-management-platform.postgresql.username" -}}
{{- if .Values.postgresql.enabled }}
{{- .Values.postgresql.auth.username }}
{{- else }}
{{- .Values.postgresql.external.username }}
{{- end }}
{{- end }}

{{/*
PostgreSQL password secret name
Uses simple naming: "amp-postgresql" instead of release-name-postgresql
*/}}
{{- define "agent-management-platform.postgresql.secretName" -}}
{{- if .Values.postgresql.enabled }}
{{- if .Values.postgresql.auth.existingSecret }}
{{- .Values.postgresql.auth.existingSecret }}
{{- else }}
{{- print "amp-postgresql" }}
{{- end }}
{{- else }}
{{- if .Values.postgresql.external.existingSecret }}
{{- .Values.postgresql.external.existingSecret }}
{{- else }}
{{- print "amp-postgresql-external" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
PostgreSQL password secret key
*/}}
{{- define "agent-management-platform.postgresql.secretPasswordKey" -}}
{{- if .Values.postgresql.enabled }}
{{- print "password" }}
{{- else }}
{{- .Values.postgresql.external.existingSecretPasswordKey | default "password" }}
{{- end }}
{{- end }}

{{/*
==============================================
Image Pull Secrets
==============================================
*/}}

{{/*
Image pull secrets
*/}}
{{- define "agent-management-platform.imagePullSecrets" -}}
{{- if .Values.global.imagePullSecrets }}
imagePullSecrets:
{{- range .Values.global.imagePullSecrets }}
  - name: {{ . }}
{{- end }}
{{- end }}
{{- end }}
