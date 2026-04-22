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
Defaults to the release name when gateway.name is not explicitly set.
*/}}
{{- define "wso2-amp-gateway-extension.apiGatewayName" -}}
{{- if .Values.gateway.name }}
{{- .Values.gateway.name }}
{{- else }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Bootstrap resource name prefix. All bootstrap resources (ServiceAccount, Role,
RoleBinding, Job) are derived from this to avoid collisions across releases.
*/}}
{{- define "wso2-amp-gateway-extension.bootstrapName" -}}
{{- printf "%s-bootstrap" .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Resolve the IDP client ID from secret or direct value
*/}}
{{- define "wso2-amp-gateway-extension.idpClientIdEnv" -}}
{{- if .Values.agentManager.idp.existingSecret }}
- name: IDP_CLIENT_ID
  valueFrom:
    secretKeyRef:
      name: {{ .Values.agentManager.idp.existingSecret }}
      key: {{ .Values.agentManager.idp.existingSecretClientIdKey }}
{{- else }}
- name: IDP_CLIENT_ID
  value: {{ .Values.agentManager.idp.clientId | quote }}
{{- end }}
{{- end }}

{{/*
Resolve the IDP client secret from secret or direct value
*/}}
{{- define "wso2-amp-gateway-extension.idpClientSecretEnv" -}}
{{- if .Values.agentManager.idp.existingSecret }}
- name: IDP_CLIENT_SECRET
  valueFrom:
    secretKeyRef:
      name: {{ .Values.agentManager.idp.existingSecret }}
      key: {{ .Values.agentManager.idp.existingSecretClientSecretKey }}
{{- else }}
- name: IDP_CLIENT_SECRET
  value: {{ .Values.agentManager.idp.clientSecret | quote }}
{{- end }}
{{- end }}
