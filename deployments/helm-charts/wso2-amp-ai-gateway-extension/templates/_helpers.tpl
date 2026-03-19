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
Name of the K8s Secret holding the gateway registration token (created by bootstrap job)
*/}}
{{- define "wso2-amp-gateway-extension.tokenSecretName" -}}
{{- .Values.gateway.tokenSecret.name | default "amp-ai-gateway-token" }}
{{- end }}

{{/*
Name of the ConfigMap holding gateway Helm values (referenced by APIGateway CR configRef)
*/}}
{{- define "wso2-amp-gateway-extension.configMapName" -}}
{{- .Values.apiGateway.config.configMapName | default "amp-ai-gateway-config" }}
{{- end }}

{{/*
Name of the APIGateway CR
*/}}
{{- define "wso2-amp-gateway-extension.apiGatewayName" -}}
{{- .Values.gateway.name | default "ai-gateway" }}
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
