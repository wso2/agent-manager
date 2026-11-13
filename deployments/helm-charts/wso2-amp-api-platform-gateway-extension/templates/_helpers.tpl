{{/*
Expand the name of the chart.
*/}}
{{- define "wso2-amp-gateway-extension.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "wso2-amp-gateway-extension.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{ include "wso2-amp-gateway-extension.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "wso2-amp-gateway-extension.selectorLabels" -}}
app.kubernetes.io/name: {{ include "wso2-amp-gateway-extension.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Name of the K8s Secret holding the gateway registration token (created by bootstrap job).
Defaults to "<release-name>-token" when not explicitly set.
*/}}
{{- define "wso2-amp-gateway-extension.tokenSecretName" -}}
{{- if .Values.gateway.tokenSecret.name }}
{{- .Values.gateway.tokenSecret.name }}
{{- else }}
{{- printf "%s-token" .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Name of the ConfigMap holding gateway Helm values (referenced by APIGateway CR configRef).
Defaults to "<release-name>-config" when not explicitly set.
*/}}
{{- define "wso2-amp-gateway-extension.configMapName" -}}
{{- if .Values.apiGateway.config.configMapName }}
{{- .Values.apiGateway.config.configMapName }}
{{- else }}
{{- printf "%s-config" .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Name of the APIGateway CR.
Defaults to "api-platform-<orgName>-<environment>" when gateway.name is not explicitly set.
*/}}
{{- define "wso2-amp-gateway-extension.apiGatewayName" -}}
{{- if .Values.gateway.name }}
{{- .Values.gateway.name }}
{{- else }}
{{- printf "api-platform-%s-%s" .Values.agentManager.orgName .Values.gateway.environment | lower | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Hostname for routing traffic through kgateway to this gateway.
Defaults to "<environment>-<orgName>.gateway.localhost" when not explicitly set.
*/}}
{{- define "wso2-amp-gateway-extension.gatewayHostname" -}}
{{- if .Values.gateway.hostname }}
{{- .Values.gateway.hostname }}
{{- else }}
{{- printf "%s-%s.gateway.localhost" .Values.gateway.environment .Values.agentManager.orgName | lower | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Bare hostname MCP proxies are publicly served on — the mcp-auth policy's
gatewayhost system parameter (the policy adds scheme and request port itself).
Derived from gateway.vhost so OAuth challenge URLs agree with the Thunder
resource-server identifiers AMS registers from the same vhost; falls back to
the routed kgateway hostname.
*/}}
{{- define "wso2-amp-gateway-extension.mcpGatewayHost" -}}
{{- $vhost := .Values.gateway.vhost | default "" | trim -}}
{{- if $vhost -}}
{{- if not (contains "://" $vhost) -}}{{- $vhost = printf "https://%s" $vhost -}}{{- end -}}
{{- regexReplaceAll ":[0-9]+$" (urlParse $vhost).host "" -}}
{{- else -}}
{{- include "wso2-amp-gateway-extension.gatewayHostname" . -}}
{{- end -}}
{{- end }}

{{/*
Label value used to bind a RestApi to its APIGateway via the
`gateway.api-platform.wso2.com/restapi-target` label/selector. The APIGateway
name is already unique per environment (see apiGatewayName), so we reuse it
verbatim rather than re-appending the environment.
*/}}
{{- define "wso2-amp-gateway-extension.restApiTarget" -}}
{{- include "wso2-amp-gateway-extension.apiGatewayName" . }}
{{- end }}

{{/*
Bootstrap resource name prefix. All bootstrap resources (ServiceAccount, Role,
RoleBinding, Job) are derived from this to avoid collisions across releases.
*/}}
{{- define "wso2-amp-gateway-extension.bootstrapName" -}}
{{- printf "%s-bootstrap" .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
In-cluster base URL of this release's gateway runtime, POSTed to AMS at registration and
stored verbatim. AMS no longer derives this from the gateway name, so this template is the
only producer. `-gw-gateway-gateway-runtime` and port 22893 are the gateway-operator's
Service convention — the same pair gateway-kgateway-route.yaml already depends on, and this
chart ships in lockstep with that operator where AMS config did not.
*/}}
{{- define "wso2-amp-gateway-extension.runtimeUrl" -}}
{{- if .Values.gateway.runtimeUrl }}
{{- /* AMS rejects any path, "/" included, so an override's trailing slash never reaches it. */}}
{{- .Values.gateway.runtimeUrl | trimSuffix "/" }}
{{- else }}
{{- printf "http://%s-gw-gateway-gateway-runtime.%s:22893" (include "wso2-amp-gateway-extension.apiGatewayName" .) .Values.apiGateway.namespace }}
{{- end }}
{{- end }}

{{/*
Resolve the IDP client ID from secret or direct value
*/}}
{{- define "wso2-amp-gateway-extension.idpClientIdEnv" -}}
{{- if or .Values.agentManager.idp.existingSecret (not .Values.agentManager.idp.clientSecret) }}
- name: IDP_CLIENT_ID
  valueFrom:
    secretKeyRef:
      name: {{ .Values.agentManager.idp.existingSecret | default "gateway-idp-credentials" | quote }}
      key: {{ required "agentManager.idp.existingSecretClientIdKey is required when using existingSecret" .Values.agentManager.idp.existingSecretClientIdKey }}
{{- else }}
- name: IDP_CLIENT_ID
  value: {{ .Values.agentManager.idp.clientId | quote }}
{{- end }}
{{- end }}

{{/*
Resolve the IDP client secret from secret or direct value
*/}}
{{- define "wso2-amp-gateway-extension.idpClientSecretEnv" -}}
{{- if or .Values.agentManager.idp.existingSecret (not .Values.agentManager.idp.clientSecret) }}
- name: IDP_CLIENT_SECRET
  valueFrom:
    secretKeyRef:
      name: {{ .Values.agentManager.idp.existingSecret | default "gateway-idp-credentials" | quote }}
      key: {{ required "agentManager.idp.existingSecretClientSecretKey is required when using existingSecret" .Values.agentManager.idp.existingSecretClientSecretKey }}
{{- else }}
- name: IDP_CLIENT_SECRET
  value: {{ .Values.agentManager.idp.clientSecret | quote }}
{{- end }}
{{- end }}
