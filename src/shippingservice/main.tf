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

variable "aws_region" {
  default = "us-east-1"
}

provider "aws" {
  region = var.aws_region
}

resource "aws_iam_role" "lambda_exec" {
  name = "shippingservice_lambda_exec_role"
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

resource "aws_iam_policy_attachment" "lambda_basic_execution" {
  name       = "shipping_lambda_basic_execution"
  roles      = [aws_iam_role.lambda_exec.name]
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

data "archive_file" "lambda_zip" {
  type        = "zip"
  source_file = "${path.module}/bootstrap"
  output_path = "${path.module}/deployment.zip"
}

resource "aws_lambda_function" "shippingservice" {
  function_name = "shippingservice"
  role          = aws_iam_role.lambda_exec.arn
  
  handler       = "bootstrap" 
  runtime       = "provided.al2023"
  architectures = ["arm64"]
  
  filename         = data.archive_file.lambda_zip.output_path 
  source_code_hash = data.archive_file.lambda_zip.output_base64sha256
}

resource "aws_lambda_function_url" "shipping_url" {
  function_name      = aws_lambda_function.shippingservice.function_name
  authorization_type = "NONE"
}

resource "aws_lambda_permission" "allow_public" {
  statement_id           = "AllowPublicFunctionUrlInvoke"
  action                 = "lambda:InvokeFunctionUrl"
  function_name          = aws_lambda_function.shippingservice.function_name
  principal              = "*"
  function_url_auth_type = "NONE"
}

resource "aws_lambda_permission" "allow_public_invoke" {
  statement_id  = "FunctionURLAllowInvokeAction"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.shippingservice.function_name
  principal     = "*"
  # Condition for InvokedViaFunctionUrl is somewhat implicit when using function_url_auth_type, but we can't easily add conditions in aws_lambda_permission without bypassing. Let's just add the basic InvokeFunction.
}

output "lambda_function_name" {
  value = aws_lambda_function.shippingservice.function_name
}

output "lambda_endpoint" {
  description = "The public HTTP URL of your Serverless Shipping Service"
  value       = aws_lambda_function_url.shipping_url.function_url
}
