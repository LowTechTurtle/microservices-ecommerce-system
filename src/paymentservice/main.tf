terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

variable "aws_region" {
  default = "us-east-1"
}

variable "event_bus_name" {
  description = "EventBridge bus name for publishing PaymentSucceeded/Failed events."
  default     = "default"
}

provider "aws" {
  region = var.aws_region
}

data "aws_region" "current" {}
data "aws_caller_identity" "current" {}

# ==========================================
# SECRETS — Stripe keys in SSM (never env vars in plain text)
# Store values manually: aws ssm put-parameter --name /paymentservice/stripe_secret_key ...
# ==========================================
data "aws_ssm_parameter" "stripe_secret_key" {
  name            = "/paymentservice/stripe_secret_key"
  with_decryption = true
}

data "aws_ssm_parameter" "stripe_webhook_secret" {
  name            = "/paymentservice/stripe_webhook_secret"
  with_decryption = true
}

# ==========================================
# DYNAMODB — Payments table
# ==========================================
resource "aws_dynamodb_table" "payments" {
  name         = "Payments"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "paymentId"

  attribute {
    name = "paymentId"
    type = "S"
  }
  attribute {
    name = "orderId"
    type = "S"
  }
  attribute {
    name = "stripePaymentIntentId"
    type = "S"
  }

  global_secondary_index {
    name            = "orderId-index"
    hash_key        = "orderId"
    projection_type = "ALL"
  }

  global_secondary_index {
    name            = "stripePaymentIntentId-index"
    hash_key        = "stripePaymentIntentId"
    projection_type = "ALL"
  }
}

# ==========================================
# IAM — shared DynamoDB policy document
# ==========================================
data "aws_iam_policy_document" "payments_dynamodb" {
  statement {
    effect = "Allow"
    actions = [
      "dynamodb:PutItem",
      "dynamodb:GetItem",
      "dynamodb:UpdateItem",
      "dynamodb:Query",
      "dynamodb:DescribeTable",
    ]
    resources = [
      aws_dynamodb_table.payments.arn,
      "${aws_dynamodb_table.payments.arn}/index/*",
    ]
  }
}

# ==========================================
# MAIN LAMBDA — handles action-style invocations from checkoutservice
# ==========================================
resource "aws_iam_role" "payment_exec" {
  name = "paymentservice_lambda_exec_role"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action    = "sts:AssumeRole"
      Effect    = "Allow"
      Principal = { Service = "lambda.amazonaws.com" }
    }]
  })
}

resource "aws_iam_policy_attachment" "payment_basic_exec" {
  name       = "payment_basic_exec"
  roles      = [aws_iam_role.payment_exec.name]
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

resource "aws_iam_role_policy" "payment_inline" {
  name = "paymentservice_inline"
  role = aws_iam_role.payment_exec.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      jsondecode(data.aws_iam_policy_document.payments_dynamodb.json).Statement[0],
      {
        Effect   = "Allow"
        Action   = "events:PutEvents"
        Resource = "arn:aws:events:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:event-bus/${var.event_bus_name}"
      },
      {
        Effect   = "Allow"
        Action   = ["ssm:GetParameter"]
        Resource = [
          data.aws_ssm_parameter.stripe_secret_key.arn,
          data.aws_ssm_parameter.stripe_webhook_secret.arn,
        ]
      }
    ]
  })
}

resource "aws_lambda_function" "paymentservice" {
  function_name    = "paymentservice"
  role             = aws_iam_role.payment_exec.arn
  handler          = "bootstrap"
  runtime          = "provided.al2023"
  filename         = "deployment.zip"
  source_code_hash = filebase64sha256("deployment.zip")

  environment {
    variables = {
      DYNAMODB_TABLE_NAME = aws_dynamodb_table.payments.name
      EVENT_BUS_NAME      = var.event_bus_name
      STRIPE_SECRET_KEY   = data.aws_ssm_parameter.stripe_secret_key.value
    }
  }
}

# ==========================================
# WEBHOOK LAMBDA — Stripe → API Gateway → Lambda
# ==========================================
resource "aws_iam_role" "webhook_exec" {
  name = "paymentservice_webhook_exec_role"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action    = "sts:AssumeRole"
      Effect    = "Allow"
      Principal = { Service = "lambda.amazonaws.com" }
    }]
  })
}

resource "aws_iam_policy_attachment" "webhook_basic_exec" {
  name       = "webhook_basic_exec"
  roles      = [aws_iam_role.webhook_exec.name]
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

resource "aws_iam_role_policy" "webhook_inline" {
  name = "webhook_inline"
  role = aws_iam_role.webhook_exec.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      jsondecode(data.aws_iam_policy_document.payments_dynamodb.json).Statement[0],
      {
        Effect   = "Allow"
        Action   = "events:PutEvents"
        Resource = "arn:aws:events:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:event-bus/${var.event_bus_name}"
      },
      {
        Effect   = "Allow"
        Action   = ["ssm:GetParameter"]
        Resource = [
          data.aws_ssm_parameter.stripe_secret_key.arn,
          data.aws_ssm_parameter.stripe_webhook_secret.arn,
        ]
      }
    ]
  })
}

resource "aws_lambda_function" "webhook" {
  function_name    = "paymentservice-webhook"
  role             = aws_iam_role.webhook_exec.arn
  handler          = "bootstrap"
  runtime          = "provided.al2023"
  filename         = "webhook.zip"
  source_code_hash = filebase64sha256("webhook.zip")

  environment {
    variables = {
      DYNAMODB_TABLE_NAME    = aws_dynamodb_table.payments.name
      EVENT_BUS_NAME         = var.event_bus_name
      STRIPE_SECRET_KEY      = data.aws_ssm_parameter.stripe_secret_key.value
      STRIPE_WEBHOOK_SECRET  = data.aws_ssm_parameter.stripe_webhook_secret.value
    }
  }
}

# Public HTTPS endpoint for the webhook Lambda — this URL goes into the
# Stripe Dashboard under Developers → Webhooks → Add endpoint.
resource "aws_lambda_function_url" "webhook_url" {
  function_name      = aws_lambda_function.webhook.function_name
  authorization_type = "NONE"
}

# ==========================================
# OUTPUTS
# ==========================================
output "payment_function_name" {
  value       = aws_lambda_function.paymentservice.function_name
  description = "Set PAYMENT_FUNCTION_NAME in checkoutservice to this value."
}

output "webhook_url" {
  value       = aws_lambda_function_url.webhook_url.function_url
  description = "Register this URL in the Stripe Dashboard as your webhook endpoint."
}
