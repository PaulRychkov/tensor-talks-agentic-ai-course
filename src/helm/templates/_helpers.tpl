{{/*
Расширяет имя чарта.
*/}}
{{- define "tensor-talks.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Создает полное имя приложения по умолчанию.
Обрезаем до 63 символов, так как некоторые поля Kubernetes ограничены этим (по спецификации DNS).
Если имя release содержит имя чарта, оно будет использовано как полное имя.
*/}}
{{- define "tensor-talks.fullname" -}}
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

{{/*
Создает имя чарта и версию, используемые в метке чарта.
*/}}
{{- define "tensor-talks.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Общие метки
*/}}
{{- define "tensor-talks.labels" -}}
helm.sh/chart: {{ include "tensor-talks.chart" . }}
{{ include "tensor-talks.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
team: tensor-talks
project: tensor-talks
{{- end -}}

{{/*
Метки селектора
*/}}
{{- define "tensor-talks.selectorLabels" -}}
app.kubernetes.io/name: {{ include "tensor-talks.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Имя сервиса для конкретного сервиса
*/}}
{{- define "tensor-talks.serviceName" -}}
{{- printf "%s-%s" (include "tensor-talks.fullname" .) .serviceName | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Создает имя service account для использования
*/}}
{{- define "tensor-talks.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "tensor-talks.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/*
Строка подключения к PostgreSQL
*/}}
{{- define "tensor-talks.postgresDSN" -}}
{{- printf "host=%s port=%d user=%s password=%s dbname=%s sslmode=%s" .Values.postgresql.host .Values.postgresql.port .Values.postgresql.user .Values.postgresql.password .Values.postgresql.database .Values.postgresql.sslMode -}}
{{- end -}}

{{/*
Строка подключения к PostgreSQL со схемой
*/}}
{{- define "tensor-talks.postgresDSNWithSchema" -}}
{{- $dsn := include "tensor-talks.postgresDSN" . -}}
{{- if .schema -}}
{{- printf "%s search_path=%s" $dsn .schema -}}
{{- else -}}
{{- $dsn -}}
{{- end -}}
{{- end -}}

{{/*
Топик Kafka с префиксом
*/}}
{{- define "tensor-talks.kafkaTopic" -}}
{{- printf "%s%s" .Values.kafka.topicPrefix .topic -}}
{{- end -}}

{{/*
Определяет imageRegistry на основе localDevelopment
*/}}
{{- define "tensor-talks.imageRegistry" -}}
{{- if .localDevelopment -}}
{{- "" -}}
{{- else -}}
{{- .global.imageRegistry | default "ghcr.io" -}}
{{- end -}}
{{- end -}}

{{/*
Определяет imagePullPolicy на основе localDevelopment
*/}}
{{- define "tensor-talks.imagePullPolicy" -}}
{{- if .localDevelopment -}}
{{- "Never" -}}
{{- else -}}
{{- "IfNotPresent" -}}
{{- end -}}
{{- end -}}

{{/*
Полный путь к Docker образу с учетом registry
Автоматически использует правильный registry на основе localDevelopment
*/}}
{{- define "tensor-talks.image" -}}
{{- $registry := include "tensor-talks.imageRegistry" .Values -}}
{{- if $registry -}}
{{- printf "%s/%s:%s" $registry .repository .tag -}}
{{- else -}}
{{- printf "%s:%s" .repository .tag -}}
{{- end -}}
{{- end -}}

