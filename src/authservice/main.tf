terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    archive = {
      source  = "hashicorp/archive"
      version = "~> 2.0"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "us-east-1"
}

variable "google_client_id" {
  description = "Google OAuth Client ID"
  type        = string
  default     = ""
}

variable "google_client_secret" {
  description = "Google OAuth Client Secret"
  type        = string
  default     = ""
  sensitive   = true
}

variable "jwt_secret" {
  description = "JWT Secret for token signing"
  type        = string
  default     = "change-this-secret-in-production"
  sensitive   = true
}

# ========================================
# DynamoDB Table for Users
# ========================================
resource "aws_dynamodb_table" "users" {
  name         = "Users"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "user_id"

  attribute {
    name = "user_id"
    type = "S"
  }

  attribute {
    name = "google_id"
    type = "S"
  }

  attribute {
    name = "email"
    type = "S"
  }

  # Global Secondary Index for google_id lookup
  global_secondary_index {
    name            = "google_id-index"
    hash_key        = "google_id"
    projection_type = "ALL"
  }

  # Global Secondary Index for email lookup
  global_secondary_index {
    name            = "email-index"
    hash_key        = "email"
    projection_type = "ALL"
  }

  tags = {
    Name        = "Users"
    Service     = "authservice"
    Environment = "production"
  }
}

# ========================================
# IAM Role for Auth Service Lambda
# ========================================
resource "aws_iam_role" "authservice_role" {
  name = "authservice-lambda-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "lambda.amazonaws.com"
        }
      }
    ]
  })
}

# CloudWatch Logs permissions
resource "aws_iam_role_policy_attachment" "authservice_basic_execution" {
  role       = aws_iam_role.authservice_role.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

# DynamoDB permissions
resource "aws_iam_policy" "authservice_dynamodb_policy" {
  name        = "authservice-dynamodb-policy"
  description = "Allow Auth Service to access Users DynamoDB table"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "dynamodb:GetItem",
          "dynamodb:PutItem",
          "dynamodb:UpdateItem",
          "dynamodb:Query",
          "dynamodb:Scan"
        ]
        Resource = [
          aws_dynamodb_table.users.arn,
          "${aws_dynamodb_table.users.arn}/index/*"
        ]
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "authservice_dynamodb_attachment" {
  role       = aws_iam_role.authservice_role.name
  policy_arn = aws_iam_policy.authservice_dynamodb_policy.arn
}

# ========================================
# Lambda Function for Auth Service
# ========================================
resource "aws_lambda_function" "authservice" {
  filename         = "deployment.zip"
  function_name    = "authservice"
  role            = aws_iam_role.authservice_role.arn
  handler         = "bootstrap"
  runtime         = "provided.al2023"
  architectures   = ["arm64"]
  timeout         = 30
  memory_size     = 256
  source_code_hash = filebase64sha256("deployment.zip")

  environment {
    variables = {
      DYNAMODB_TABLE_NAME   = aws_dynamodb_table.users.name
      GOOGLE_CLIENT_ID      = var.google_client_id
      GOOGLE_CLIENT_SECRET  = var.google_client_secret
      JWT_SECRET            = var.jwt_secret
    }
  }

  depends_on = [
    aws_iam_role_policy_attachment.authservice_basic_execution,
    aws_iam_role_policy_attachment.authservice_dynamodb_attachment
  ]
}

# ========================================
# Lambda Function URL
# ========================================
resource "aws_lambda_function_url" "authservice" {
  function_name      = aws_lambda_function.authservice.function_name
  authorization_type = "NONE"

  cors {
    allow_origins     = ["*"]
    allow_methods     = ["*"]
    allow_headers     = ["content-type", "authorization"]
    expose_headers    = ["content-type"]
    max_age          = 86400
  }
}

# ========================================
# Outputs
# ========================================
output "auth_function_name" {
  description = "Auth Service Lambda function name"
  value       = aws_lambda_function.authservice.function_name
}

output "auth_function_url" {
  description = "Auth Service Function URL"
  value       = aws_lambda_function_url.authservice.function_url
}

output "users_table_name" {
  description = "DynamoDB Users table name"
  value       = aws_dynamodb_table.users.name
}

output "google_login_url" {
  description = "Google OAuth login URL"
  value       = "${aws_lambda_function_url.authservice.function_url}auth/google"
}
