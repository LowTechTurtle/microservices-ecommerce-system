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

variable "cart_function_name" {
  default = "cartservice"
}

variable "catalog_function_name" {
  default = "productcatalogservice"
}

variable "payment_function_name" {
  description = "Set to the paymentservice Lambda name once deployed; leave empty to use the stub client."
  default     = ""
}

provider "aws" {
  region = var.aws_region
}

data "aws_region" "current" {}
data "aws_caller_identity" "current" {}

# ==========================================
# DYNAMODB — Orders table
# ==========================================
resource "aws_dynamodb_table" "orders" {
  name         = "Orders"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "orderId"

  attribute {
    name = "orderId"
    type = "S"
  }

  attribute {
    name = "userId"
    type = "S"
  }

  global_secondary_index {
    name            = "userId-index"
    hash_key        = "userId"
    projection_type = "ALL"
  }
}

# ==========================================
# IAM ROLE — checkout Lambda
# ==========================================
resource "aws_iam_role" "checkout_exec" {
  name = "checkoutservice_lambda_exec_role"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action    = "sts:AssumeRole"
      Effect    = "Allow"
      Principal = { Service = "lambda.amazonaws.com" }
    }]
  })
}

resource "aws_iam_policy_attachment" "checkout_basic_execution" {
  name       = "checkout_lambda_basic_execution"
  roles      = [aws_iam_role.checkout_exec.name]
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

resource "aws_iam_role_policy" "checkout_inline" {
  name = "checkoutservice_inline"
  role = aws_iam_role.checkout_exec.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "dynamodb:PutItem",
          "dynamodb:GetItem",
          "dynamodb:UpdateItem",
          "dynamodb:Query",
          "dynamodb:DescribeTable"
        ]
        Resource = [
          aws_dynamodb_table.orders.arn,
          "${aws_dynamodb_table.orders.arn}/index/*"
        ]
      },
      {
        Effect = "Allow"
        Action = "lambda:InvokeFunction"
        Resource = [
          "arn:aws:lambda:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:function:${var.cart_function_name}",
          "arn:aws:lambda:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:function:${var.catalog_function_name}",
          "arn:aws:lambda:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:function:${var.payment_function_name}"
        ]
      }
    ]
  })
}

# ==========================================
# CHECKOUT LAMBDA (HTTP API)
# ==========================================
resource "aws_lambda_function" "checkoutservice" {
  function_name = "checkoutservice"
  role          = aws_iam_role.checkout_exec.arn
  handler       = "bootstrap"
  runtime       = "provided.al2023"
  filename      = "deployment.zip"
  source_code_hash = filebase64sha256("deployment.zip")

  environment {
    variables = {
      DYNAMODB_TABLE_NAME    = aws_dynamodb_table.orders.name
      CART_FUNCTION_NAME     = var.cart_function_name
      CATALOG_FUNCTION_NAME  = var.catalog_function_name
      PAYMENT_FUNCTION_NAME  = var.payment_function_name
    }
  }
}

resource "aws_lambda_function_url" "checkoutservice_url" {
  function_name      = aws_lambda_function.checkoutservice.function_name
  authorization_type = "NONE" # tighten before production (IAM, JWT, or API Gateway authorizer)
}

# ==========================================
# PAYMENT EVENT CONSUMER LAMBDA + EventBridge rule
# Triggered by paymentservice publishing to the default event bus.
# ==========================================
resource "aws_iam_role" "payment_consumer_exec" {
  name = "checkoutservice_payment_consumer_role"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action    = "sts:AssumeRole"
      Effect    = "Allow"
      Principal = { Service = "lambda.amazonaws.com" }
    }]
  })
}

resource "aws_iam_policy_attachment" "payment_consumer_basic_execution" {
  name       = "payment_consumer_basic_execution"
  roles      = [aws_iam_role.payment_consumer_exec.name]
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

resource "aws_iam_role_policy" "payment_consumer_inline" {
  name = "payment_consumer_inline"
  role = aws_iam_role.payment_consumer_exec.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "dynamodb:GetItem",
          "dynamodb:UpdateItem"
        ]
        Resource = aws_dynamodb_table.orders.arn
      },
      {
        Effect   = "Allow"
        Action   = "lambda:InvokeFunction"
        Resource = "arn:aws:lambda:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:function:${var.cart_function_name}"
      }
    ]
  })
}

resource "aws_lambda_function" "payment_consumer" {
  function_name    = "checkoutservice-payment-consumer"
  role             = aws_iam_role.payment_consumer_exec.arn
  handler          = "bootstrap"
  runtime          = "provided.al2023"
  filename         = "paymentconsumer.zip"
  source_code_hash = filebase64sha256("paymentconsumer.zip")

  environment {
    variables = {
      DYNAMODB_TABLE_NAME = aws_dynamodb_table.orders.name
      CART_FUNCTION_NAME  = var.cart_function_name
    }
  }
}

resource "aws_cloudwatch_event_rule" "payment_events" {
  name        = "paymentservice-events"
  description = "Routes PaymentSucceeded / PaymentFailed events to the checkout consumer."
  event_pattern = jsonencode({
    source      = ["paymentservice"]
    detail-type = ["PaymentSucceeded", "PaymentFailed"]
  })
}

resource "aws_cloudwatch_event_target" "payment_to_consumer" {
  rule      = aws_cloudwatch_event_rule.payment_events.name
  target_id = "checkoutPaymentConsumer"
  arn       = aws_lambda_function.payment_consumer.arn
}

resource "aws_lambda_permission" "allow_eventbridge" {
  statement_id  = "AllowExecutionFromEventBridge"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.payment_consumer.function_name
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.payment_events.arn
}

# ==========================================
# OUTPUTS
# ==========================================
output "checkout_function_name" {
  value = aws_lambda_function.checkoutservice.function_name
}

output "checkout_function_url" {
  value = aws_lambda_function_url.checkoutservice_url.function_url
}

output "payment_consumer_function_name" {
  value = aws_lambda_function.payment_consumer.function_name
}
