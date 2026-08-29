{{/*
Selector labels shared by the Deployment and every Service that targets it.
*/}}
{{- define "axiom.selectorLabels" -}}
app: zyvor-axiom
{{- end }}

{{- define "axiom.image" -}}
{{- printf "%s:%s" .Values.image.repository (.Values.image.tag | default .Chart.AppVersion) -}}
{{- end }}

{{- define "axiom.tlsSecretName" -}}
{{- .Values.tls.existingSecret | default "zyvor-axiom-tls" -}}
{{- end }}
