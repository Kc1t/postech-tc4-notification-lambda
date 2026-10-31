locals {
  name = "${var.project_name}-${var.environment}-notifier"
  tags = {
    Project     = var.project_name
    Environment = var.environment
    Service     = "notification"
    ManagedBy   = "terraform"
  }
  new_relic_enabled = var.new_relic_license_key != ""
}

data "aws_iam_role" "lab" {
  name = var.lab_role_name
}

data "aws_sqs_queue" "notifications" {
  name = var.queue_name
}

# Idempotência: um item por message_id já notificado; o TTL limpa os antigos.
resource "aws_dynamodb_table" "dedup" {
  name         = "${local.name}-dedup"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "message_id"

  attribute {
    name = "message_id"
    type = "S"
  }

  ttl {
    attribute_name = "expires_at"
    enabled        = true
  }

  point_in_time_recovery {
    enabled = false
  }

  tags = local.tags
}

resource "aws_cloudwatch_log_group" "notifier" {
  name              = "/aws/lambda/${local.name}"
  retention_in_days = var.log_retention_days
  tags              = local.tags
}

resource "aws_lambda_function" "notifier" {
  function_name = local.name
  description   = "Notifica o cliente por e-mail a cada mudança de status da OS"
  role          = data.aws_iam_role.lab.arn

  filename         = var.artifact_path
  source_code_hash = filebase64sha256(var.artifact_path)

  runtime       = "provided.al2023"
  handler       = "bootstrap"
  architectures = ["arm64"]
  timeout       = 30
  memory_size   = 128
  layers        = local.new_relic_enabled ? [var.new_relic_layer_arn] : []

  environment {
    variables = merge({
      SMTP_HOST     = var.smtp_host
      SMTP_PORT     = var.smtp_port
      SMTP_USERNAME = var.smtp_username
      SMTP_PASSWORD = var.smtp_password
      SMTP_FROM     = var.smtp_from
      DEDUP_TABLE   = aws_dynamodb_table.dedup.name
      PORTAL_URL    = var.portal_url
      LOG_LEVEL     = "info"
      }, local.new_relic_enabled ? {
      NEW_RELIC_ACCOUNT_ID                   = var.new_relic_account_id
      NEW_RELIC_TRUSTED_ACCOUNT_KEY          = var.new_relic_account_id
      NEW_RELIC_LICENSE_KEY                  = var.new_relic_license_key
      NEW_RELIC_LAMBDA_EXTENSION_ENABLED     = "true"
      NEW_RELIC_EXTENSION_SEND_FUNCTION_LOGS = "true"
      NEW_RELIC_APM_LAMBDA_MODE              = "true"
      NR_TAGS                                = "NR.Apm.Lambda.Mode:true;Service:notification;Environment:${var.environment}"
    } : {})
  }

  depends_on = [aws_cloudwatch_log_group.notifier]
  tags       = local.tags
}

# A fila tem DLQ (maxReceiveCount 5) definida no repositório de infra. Com
# ReportBatchItemFailures, só as mensagens que falharam voltam para a fila.
resource "aws_lambda_event_source_mapping" "queue" {
  event_source_arn                   = data.aws_sqs_queue.notifications.arn
  function_name                      = aws_lambda_function.notifier.arn
  batch_size                         = var.batch_size
  maximum_batching_window_in_seconds = 0
  function_response_types            = ["ReportBatchItemFailures"]

  scaling_config {
    maximum_concurrency = 5
  }
}
