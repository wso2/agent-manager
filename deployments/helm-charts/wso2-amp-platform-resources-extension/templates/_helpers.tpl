{{/*
Expand the name of the chart.
*/}}
{{- define "amp-build-extension.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "amp-build-extension.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
These labels should be applied to all resources and include:
- helm.sh/chart: Chart name and version
- app.kubernetes.io/name: Name of the application
- app.kubernetes.io/instance: Unique name identifying the instance of an application
- app.kubernetes.io/version: Current version of the application
- app.kubernetes.io/managed-by: Tool being used to manage the application
- app.kubernetes.io/part-of: Name of a higher level application this one is part of
*/}}
{{- define "amp-build-extension.labels" -}}
helm.sh/chart: {{ include "amp-build-extension.chart" . }}
{{ include "amp-build-extension.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: openchoreo
{{- with .Values.global.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Selector labels
These labels are used for pod selectors and should be stable across upgrades.
They should NOT include version or chart labels as these change with upgrades.
*/}}
{{- define "amp-build-extension.selectorLabels" -}}
app.kubernetes.io/name: {{ include "amp-build-extension.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Get the registry endpoint for workflow templates
Returns external endpoint if global.baseDomain is set, otherwise uses configured endpoint
*/}}
{{- define "openchoreo-workflow-plane.registryEndpoint" -}}
{{- if .Values.global.baseDomain -}}
  {{- printf "registry.%s" .Values.global.baseDomain -}}
{{- else -}}
  {{- .Values.global.registry.endpoint -}}
{{- end -}}
{{- end -}}

{{/*
Get buildpack image by ID
Returns the appropriate image reference based on buildpackCache.enabled setting.
When caching is enabled, returns the cached image path prefixed with registry endpoint.
When caching is disabled, returns the remote image reference directly.

Usage:
  {{ include "openchoreo-workflow-plane.buildpackImage" (dict "id" "google-builder" "context" .) }}

Parameters:
  - id: The unique identifier of the buildpack image (e.g., "google-builder", "ballerina-run")
  - context: The Helm context (usually .)
*/}}
{{- define "openchoreo-workflow-plane.buildpackImage" -}}
{{- $id := .id -}}
{{- $ctx := .context -}}
{{- $cacheEnabled := $ctx.Values.global.defaultResources.buildpackCache.enabled -}}
{{- $registryEndpoint := include "openchoreo-workflow-plane.registryEndpoint" $ctx -}}
{{- $found := false -}}
{{- range $ctx.Values.global.defaultResources.buildpackCache.images -}}
  {{- if eq .id $id -}}
    {{- $found = true -}}
    {{- if $cacheEnabled -}}
      {{- printf "%s/%s" $registryEndpoint .cachedImage -}}
    {{- else -}}
      {{- .remoteImage -}}
    {{- end -}}
  {{- end -}}
{{- end -}}
{{- if not $found -}}
  {{- fail (printf "Buildpack image with id '%s' not found in buildpackCache.images" $id) -}}
{{- end -}}
{{- end -}}

{{/*
Namespace-qualified DNS name of the per-environment gateway runtime Service.

The Service name always follows the apiGatewayName convention
("api-platform-<org>-<env>-gw-gateway-gateway-runtime"), but the namespace it
lives in is a deployment choice: setup-gateway.sh and add-environment.sh install
the gateway extension into "<org>-<env>", while the extension chart's own
apiGateway.namespace default is openchoreo-data-plane. Callers set
apiPlatformGateway.namespace to match wherever they installed it; empty keeps the
per-org-env convention.

Emits an OpenChoreo trait placeholder expression, so ${metadata.*} is resolved by
the controller rather than by Helm.
*/}}
{{- define "amp.gatewayRuntimeHost" -}}
{{- $svc := "api-platform-${metadata.componentNamespace}-${metadata.environmentName}-gw-gateway-gateway-runtime" -}}
{{- if .Values.apiPlatformGateway.namespace -}}
{{- printf "%s.%s" $svc .Values.apiPlatformGateway.namespace -}}
{{- else -}}
{{- printf "%s.${metadata.componentNamespace}-${metadata.environmentName}" $svc -}}
{{- end -}}
{{- end -}}

{{/*
Stamped onto build-stage pods (Argo applies templates[].metadata to the pod), and the only
thing build-workflow-networkpolicy.yaml's podSelector can match — CR labels never reach the pod.
*/}}
{{- define "amp.buildWorkflowPodLabels" -}}
app.kubernetes.io/name: amp-build
app.kubernetes.io/component: build-workflow
{{- end -}}
