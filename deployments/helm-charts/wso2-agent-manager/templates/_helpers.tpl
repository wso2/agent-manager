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
{{- if .Values.serviceAccount.create }}
{{- default "amp" .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Non-empty when at least one pre-install/pre-upgrade hook Job (TLS certs,
JWT keys) will render; the hook SA and Role/RoleBinding are only needed then.
*/}}
{{- define "agent-management-platform.hookJobsEnabled" -}}
{{- if or (and .Values.jwtKeysGeneration.enabled (not .Values.jwtSigning.existingSecret)) (and .Values.agentManagerService.enabled .Values.tlsCertsGeneration.enabled (not .Values.agentManagerService.certificates.certificatesSecret)) -}}
true
{{- end -}}
{{- end }}

{{/*
ServiceAccount name for the pre-install/pre-upgrade hook Jobs (TLS certs,
JWT keys). Separate from the amp-api runtime SA so the Deployment holds no RBAC.
With create disabled an existing SA name must be supplied — there is no safe
fallback, so fail at render time instead of at the hook deadline.
*/}}
{{- define "agent-management-platform.hookServiceAccountName" -}}
{{- if (.Values.hookServiceAccount).create }}
{{- default (printf "%s-hooks" (include "agent-management-platform.fullname" .)) .Values.hookServiceAccount.name }}
{{- else }}
{{- required "hookServiceAccount.name must name an existing ServiceAccount when hookServiceAccount.create is false" (.Values.hookServiceAccount).name }}
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
Accepted token audiences: keyManager.audience plus serverPublicURL with /mcp
appended (no trailing slash — the MCP spec's canonical-URI guidance prefers
the form without one) — the RFC 8707 resource identifier Thunder stamps into
MCP tokens. nospace keeps a spaced-out list from defeating the uniq. See
values.yaml for why it is derived.
*/}}
{{- define "agent-management-platform.agentManagerService.audience" -}}
{{- $audiences := .Values.agentManagerService.config.keyManager.audience | nospace | splitList "," | compact -}}
{{- with .Values.agentManagerService.config.serverPublicURL -}}
{{- $audiences = append $audiences (printf "%s/mcp" (trimSuffix "/" .)) -}}
{{- end -}}
{{- join "," (uniq $audiences) -}}
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
PostgreSQL SSL mode (libpq "sslmode": disable|allow|prefer|require|verify-ca|verify-full)
Only meaningful for an external database — the in-cluster PostgreSQL this chart
deploys serves plaintext, so the value stays empty there and the driver keeps its
"prefer" default. Empty output means the DB_SSL_MODE env var is not rendered.
*/}}
{{- define "agent-management-platform.postgresql.sslMode" -}}
{{- if .Values.postgresql.enabled }}
{{- print "" }}
{{- else }}
{{- .Values.postgresql.external.sslMode | default "" }}
{{- end }}
{{- end }}

{{/*
PostgreSQL SSL root certificate (libpq "sslrootcert"): a path inside the
container, or the literal "system" to validate against the image trust store.
*/}}
{{- define "agent-management-platform.postgresql.sslRootCert" -}}
{{- if .Values.postgresql.enabled }}
{{- print "" }}
{{- else }}
{{- .Values.postgresql.external.sslRootCert | default "" }}
{{- end }}
{{- end }}

{{/*
Renders the optional DB TLS environment variables shared by the API deployment
and the migration job. Emits nothing when no SSL mode is configured.
*/}}
{{- define "agent-management-platform.postgresql.tlsEnv" -}}
{{- $sslMode := include "agent-management-platform.postgresql.sslMode" . }}
{{- $sslRootCert := include "agent-management-platform.postgresql.sslRootCert" . }}
{{- if $sslMode }}
- name: DB_SSL_MODE
  value: {{ $sslMode | quote }}
{{- end }}
{{- if $sslRootCert }}
- name: DB_SSL_ROOT_CERT
  value: {{ $sslRootCert | quote }}
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
JWT Keys Secret Helpers
==============================================
*/}}

{{/*
JWT Keys Secret name
*/}}
{{- define "agent-management-platform.jwtKeysSecretName" -}}
{{- if .Values.jwtSigning.existingSecret }}
{{- .Values.jwtSigning.existingSecret }}
{{- else }}
{{- printf "%s-jwt-keys" (include "agent-management-platform.fullname" .) }}
{{- end }}
{{- end }}

{{/*
TLS Certificates Secret name
*/}}
{{- define "agent-management-platform.tlsCertsSecretName" -}}
{{- if .Values.agentManagerService.certificates.certificatesSecret }}
{{- .Values.agentManagerService.certificates.certificatesSecret }}
{{- else }}
{{- printf "%s-tls-certs" (include "agent-management-platform.fullname" .) }}
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

{{/*
==============================================
Images
==============================================
*/}}

{{/*
Image reference for a first-party image.

global.ampImageRegistry, when set, replaces the registry and organization portion
of the image's repository and keeps the trailing image name, so one value
redirects every AMP image at a private registry. Left empty (the default) the
fully-qualified repository is used exactly as written, which keeps existing
installs and any per-image repository override working unchanged.

Deliberately not named global.imageRegistry: that is a Bitnami convention, and
Helm passes global values to subcharts, so the name would also rewrite the
postgresql subchart's images and trip its unrecognized-container guard.

Usage: include "agent-management-platform.image" (dict "image" .Values.console.image "ctx" .)
*/}}
{{- define "agent-management-platform.image" -}}
{{- $image := .image -}}
{{- $ctx := .ctx -}}
{{- $repository := $image.repository -}}
{{- with $ctx.Values.global.ampImageRegistry -}}
{{- $repository = printf "%s/%s" (trimSuffix "/" .) (base $repository) -}}
{{- end -}}
{{- printf "%s:%s" $repository ($image.tag | default $ctx.Chart.AppVersion) -}}
{{- end -}}

{{/*
NATS URL the service publishes events to: the configured one, else the
bundled NATS, else empty (events off).
*/}}
{{- define "agent-management-platform.natsUrl" -}}
{{- if ne (.Values.agentManagerService.config.events.mode | default "nats") "nats" -}}
{{- else if .Values.agentManagerService.config.events.natsUrl -}}
{{- .Values.agentManagerService.config.events.natsUrl -}}
{{- else if .Values.nats.enabled -}}
{{- printf "nats://amp-nats.%s.svc.cluster.local:4222" .Release.Namespace -}}
{{- end -}}
{{- end }}

{{/*
Event bus mode. "nats" with no URL and no bundled NATS has nothing to connect
to, so it renders as "off" rather than failing the API at startup.
*/}}
{{- define "agent-management-platform.eventsMode" -}}
{{- $mode := .Values.agentManagerService.config.events.mode | default "nats" -}}
{{- if and (eq $mode "nats") (not (include "agent-management-platform.natsUrl" .)) -}}
off
{{- else -}}
{{- $mode -}}
{{- end -}}
{{- end }}

{{/*
Webhook dispatcher name, and settings that fall back to the chart's PostgreSQL.
*/}}
{{- define "agent-management-platform.webhookDispatcher.fullname" -}}
amp-webhook-dispatcher
{{- end }}

{{- define "agent-management-platform.webhookDispatcher.dbHost" -}}
{{- .Values.webhookDispatcher.database.host | default (include "agent-management-platform.postgresql.host" .) -}}
{{- end }}

{{- define "agent-management-platform.webhookDispatcher.dbPort" -}}
{{- .Values.webhookDispatcher.database.port | default (include "agent-management-platform.postgresql.port" .) -}}
{{- end }}

{{- define "agent-management-platform.webhookDispatcher.dbUser" -}}
{{- .Values.webhookDispatcher.database.user | default (include "agent-management-platform.postgresql.username" .) -}}
{{- end }}

{{- define "agent-management-platform.webhookDispatcher.dbSecretName" -}}
{{- .Values.webhookDispatcher.database.existingSecret | default (include "agent-management-platform.postgresql.secretName" .) -}}
{{- end }}

{{- define "agent-management-platform.webhookDispatcher.dbSecretKey" -}}
{{- .Values.webhookDispatcher.database.existingSecretPasswordKey | default (include "agent-management-platform.postgresql.secretPasswordKey" .) -}}
{{- end }}

{{/*
Secret reference for the key the API and the dispatcher share.
*/}}
{{- define "agent-management-platform.webhookDispatcher.apiKeyRef" -}}
secretKeyRef:
  {{- if .Values.webhookDispatcher.apiKey.existingSecret }}
  name: {{ .Values.webhookDispatcher.apiKey.existingSecret }}
  key: {{ .Values.webhookDispatcher.apiKey.existingSecretKey | default "dispatcher-api-key" }}
  {{- else }}
  name: {{ include "agent-management-platform.agentManagerService.fullname" . }}
  key: dispatcher-api-key
  {{- end }}
{{- end }}

{{/*
Service mode of the API deployment.
*/}}
{{- define "agent-management-platform.serviceMode" -}}
{{- if .Values.webhookDispatcher.enabled -}}api{{- else -}}all{{- end -}}
{{- end }}
