terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    archive = {
      source  = "hashicorp/archive"
      version = "~> 2.4"
    }
  }
}

# --- VARIABLES ---

variable "aws_region" {
  default = "us-east-1" # Change to your preferred region
}

provider "aws" {
  region = var.aws_region
}

# ==========================================
# DYNAMODB TABLE ($0 Idle Cost)
# ==========================================

resource "aws_dynamodb_table" "cart_table" {
  name         = "Carts"
  # PAY_PER_REQUEST is the magic setting for $0 cost when idle
  billing_mode = "PAY_PER_REQUEST" 
  hash_key     = "user_id"

  attribute {
    name = "user_id"
    type = "S"
  }
}

# ==========================================
# IAM ROLE & POLICIES FOR LAMBDA
# ==========================================

# Allow Lambda to assume this role
resource "aws_iam_role" "lambda_exec" {
  name = "cartservice_lambda_exec_role"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement =[{
      Action = "sts:AssumeRole"
      Effect = "Allow"
      Principal = {
        Service = "lambda.amazonaws.com"
      }
    }]
  })
}

# Add basic execution permissions (CloudWatch logs)
resource "aws_iam_policy_attachment" "lambda_basic_execution" {
  name       = "lambda_basic_execution"
  roles      = [aws_iam_role.lambda_exec.name]
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

# Add DynamoDB permissions
resource "aws_iam_role_policy" "lambda_dynamodb_policy" {
  name   = "cartservice_dynamodb_access"
  role   = aws_iam_role.lambda_exec.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement =[
      {
        Effect   = "Allow"
        Action   =[
          "dynamodb:PutItem",
          "dynamodb:GetItem",
          "dynamodb:DeleteItem",
          "dynamodb:DescribeTable"
        ]
        Resource = aws_dynamodb_table.cart_table.arn
      }
    ]
  })
}

# ==========================================
# 3. PACKAGE THE COMPILED GO BINARY
# ==========================================

data "archive_file" "lambda_zip" {
  type        = "zip"
  source_file = "${path.module}/bootstrap"
  output_path = "${path.module}/deployment.zip"
}

# ==========================================
# 4. AWS LAMBDA FUNCTION ($0 Idle Cost)
# ==========================================

resource "aws_lambda_function" "cartservice" {
  function_name = "cartservice"
  role          = aws_iam_role.lambda_exec.arn
  
  # AWS strictly requires the Go binary to be named "bootstrap" for the al2023 runtime
  handler       = "bootstrap" 
  runtime       = "provided.al2023"
  architectures = ["arm64"]
  
  # Trỏ filename vào output của block archive_file ở trên
  filename         = data.archive_file.lambda_zip.output_path 
  source_code_hash = data.archive_file.lambda_zip.output_base64sha256

  environment {
    variables = {
      DYNAMODB_TABLE_NAME = aws_dynamodb_table.cart_table.name
    }
  }
}

# ==========================================
# 5. LAMBDA FUNCTION URL (HTTP Endpoint)
# ==========================================

resource "aws_lambda_function_url" "cart_url" {
  function_name      = aws_lambda_function.cartservice.function_name
  authorization_type = "NONE"
}

resource "aws_lambda_permission" "allow_public" {
  statement_id           = "AllowPublicFunctionUrlInvoke"
  action                 = "lambda:InvokeFunctionUrl"
  function_name          = aws_lambda_function.cartservice.function_name
  principal              = "*"
  function_url_auth_type = "NONE"
}

output "lambda_function_name" {
  value = aws_lambda_function.cartservice.function_name
}

output "lambda_endpoint" {
  description = "The public HTTP URL of your Serverless Cart Service"
  value       = aws_lambda_function_url.cart_url.function_url
}