{{/*
Expand the name of the chart.
*/}}
{{- define "amp-observability-extension.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "amp-observability-extension.fullname" -}}
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
{{- define "amp-observability-extension.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "amp-observability-extension.labels" -}}
helm.sh/chart: {{ include "amp-observability-extension.chart" . }}
{{ include "amp-observability-extension.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "amp-observability-extension.selectorLabels" -}}
app.kubernetes.io/name: {{ include "amp-observability-extension.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Accepted token audiences: auth.audience plus publicUrl with /mcp appended (no
trailing slash — the MCP spec's canonical-URI guidance prefers the form
without one) — the RFC 8707 resource identifier Thunder stamps into MCP
tokens. nospace keeps a spaced-out list from defeating the uniq. See
values.yaml for why it is derived.
*/}}
{{- define "amp-observability-extension.audience" -}}
{{- $audiences := .Values.amObserver.auth.audience | nospace | splitList "," | compact -}}
{{- with .Values.amObserver.publicUrl -}}
{{- $audiences = append $audiences (printf "%s/mcp" (trimSuffix "/" .)) -}}
{{- end -}}
{{- join "," (uniq $audiences) -}}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "amp-observability-extension.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "amp-observability-extension.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Image reference for the observer.

global.ampImageRegistry, when set, replaces the registry and organization
portion of the repository and keeps the image name, so one value redirects the
AMP images across all the AMP charts at a private registry. Left empty (the
default) the fully-qualified repository is used exactly as written.

Deliberately not named global.imageRegistry: that is a Bitnami convention, and
Helm passes global values into subcharts, so the name collides wherever a
Bitnami chart is a dependency.
*/}}
{{- define "amp-observability-extension.image" -}}
{{- $repository := .Values.amObserver.image.repository -}}
{{- with .Values.global.ampImageRegistry -}}
{{- $repository = printf "%s/%s" (trimSuffix "/" .) (base $repository) -}}
{{- end -}}
{{- printf "%s:%s" $repository (.Values.amObserver.image.tag | default .Chart.AppVersion) -}}
{{- end -}}

{{/*
Image pull secrets
*/}}
{{- define "amp-observability-extension.imagePullSecrets" -}}
{{- if .Values.global.imagePullSecrets }}
imagePullSecrets:
{{- range .Values.global.imagePullSecrets }}
  - name: {{ . }}
{{- end }}
{{- end }}
{{- end }}
