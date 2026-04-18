terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
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
# IAM ROLE FOR LAMBDA
# ==========================================

# 1. Allow Lambda to assume this role
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

# 2. Add basic execution permissions (CloudWatch logs)
resource "aws_iam_policy_attachment" "lambda_basic_execution" {
  name       = "lambda_basic_execution"
  roles      = [aws_iam_role.lambda_exec.name]
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

# 3. Add DynamoDB permissions
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
# AWS LAMBDA FUNCTION ($0 Idle Cost)
# ==========================================

resource "aws_lambda_function" "cartservice" {
  function_name = "cartservice"
  role          = aws_iam_role.lambda_exec.arn
  
  # AWS strictly requires the Go binary to be named "bootstrap" for the al2023 runtime
  handler       = "bootstrap" 
  runtime       = "provided.al2023"
  
  # Terraform looks for this ZIP file in your project directory
  filename      = "deployment.zip" 
  
  source_code_hash = filebase64sha256("deployment.zip")

  environment {
    variables = {
      DYNAMODB_TABLE_NAME = aws_dynamodb_table.cart_table.name
    }
  }
}

# ==========================================
# OUTPUTS
# ==========================================

output "lambda_function_name" {
  value = aws_lambda_function.cartservice.function_name
}
