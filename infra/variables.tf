variable "aws_region" {
  type    = string
  default = "us-east-1"
}

variable "project_name" {
  type    = string
  default = "postech-tc4"
}

variable "environment" {
  type = string
}

variable "lab_role_name" {
  type        = string
  default     = "LabRole"
  description = "No Learner Lab não dá para criar IAM: a function usa a LabRole"
}

variable "artifact_path" {
  type    = string
  default = "../notifier.zip"
}

variable "queue_name" {
  type        = string
  default     = "notification-os-events"
  description = "Fila assinada no tópico os-events (criada no repositório de infra)"
}

variable "batch_size" {
  type        = number
  default     = 1
  description = "1 mensagem por invocação: um trace distribuído por notificação no New Relic"
}

variable "smtp_host" {
  type    = string
  default = "smtp-relay.brevo.com"
}

variable "smtp_port" {
  type    = string
  default = "587"
}

variable "smtp_username" {
  type      = string
  sensitive = true
}

variable "smtp_password" {
  type      = string
  sensitive = true
}

variable "smtp_from" {
  type    = string
  default = "noreply@reservoirdevs.online"
}

variable "portal_url" {
  type    = string
  default = ""
}

variable "new_relic_account_id" {
  type    = string
  default = ""
}

variable "new_relic_license_key" {
  type      = string
  sensitive = true
  default   = ""
}

variable "new_relic_layer_arn" {
  type        = string
  default     = "arn:aws:lambda:us-east-1:451483290750:layer:NewRelicLambdaExtensionARM64:62"
  description = "Extension do New Relic para arm64 (provided.al2023)"
}

variable "log_retention_days" {
  type    = number
  default = 14
}
