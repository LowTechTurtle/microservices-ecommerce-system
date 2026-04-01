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

# You can change the region to your preferred AWS region
provider "aws" {
  region = "us-east-1"
}

# ------------------------------------------------------
# 1. DYNAMODB TABLE (On-Demand Capacity)
# ------------------------------------------------------
resource "aws_dynamodb_table" "products" {
  name         = "products"
  billing_mode = "PAY_PER_REQUEST" # $0 cost when idle
  hash_key     = "id"

  attribute {
    name = "id"
    type = "S"
  }

  tags = {
    Environment = "production"
    Service     = "ProductCatalog"
  }
}

# ------------------------------------------------------
# 2. IAM ROLE & POLICIES FOR LAMBDA
# ------------------------------------------------------
data "aws_iam_policy_document" "lambda_assume_role" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "lambda_exec_role" {
  name               = "ProductCatalogLambdaRole"
  assume_role_policy = data.aws_iam_policy_document.lambda_assume_role.json
}

# Basic execution role for CloudWatch logs
resource "aws_iam_role_policy_attachment" "lambda_basic_execution" {
  role       = aws_iam_role.lambda_exec_role.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

# Policy to allow scanning the DynamoDB table
data "aws_iam_policy_document" "dynamodb_scan_policy" {
  statement {
    actions = [
      "dynamodb:Scan"
    ]
    resources =[
      aws_dynamodb_table.products.arn
    ]
  }
}

resource "aws_iam_role_policy" "lambda_dynamodb_policy" {
  name   = "LambdaDynamoDBAccess"
  role   = aws_iam_role.lambda_exec_role.name
  policy = data.aws_iam_policy_document.dynamodb_scan_policy.json
}

# ------------------------------------------------------
# 3. PACKAGE THE COMPILED GO BINARY
# ------------------------------------------------------
data "archive_file" "lambda_zip" {
  type        = "zip"
  source_file = "${path.module}/bootstrap"
  output_path = "${path.module}/function.zip"
}

# ------------------------------------------------------
# 4. AWS LAMBDA FUNCTION
# ------------------------------------------------------
resource "aws_lambda_function" "product_catalog" {
  function_name    = "ProductCatalogService"
  role             = aws_iam_role.lambda_exec_role.arn
  filename         = data.archive_file.lambda_zip.output_path
  source_code_hash = data.archive_file.lambda_zip.output_base64sha256
  
  handler       = "bootstrap" # Required by TF, but ignored by custom runtimes
  runtime       = "provided.al2023"
  architectures = ["arm64"]
  timeout       = 10 # 10 seconds to allow for DB connection/scan cold start

  environment {
    variables = {
      DYNAMODB_TABLE_NAME = aws_dynamodb_table.products.name
    }
  }

  depends_on =[
    aws_iam_role_policy_attachment.lambda_basic_execution,
    aws_iam_role_policy.lambda_dynamodb_policy
  ]
}

# ------------------------------------------------------
# 5. LAMBDA FUNCTION URL (HTTP Endpoint)
# ------------------------------------------------------
resource "aws_lambda_function_url" "product_catalog_url" {
  function_name      = aws_lambda_function.product_catalog.function_name
  authorization_type = "NONE" # Publicly accessible, handled securely over HTTPS

  # Optional CORS configuration if connecting directly from a web browser frontend
  cors {
    allow_credentials = true
    allow_origins     = ["*"]
    allow_methods     = ["GET", "POST"]
    allow_headers     = ["*"]
    expose_headers    = ["*"]
    max_age           = 86400
  }
}

# ------------------------------------------------------
# 6. OUTPUTS
# ------------------------------------------------------
output "lambda_endpoint" {
  description = "The public HTTP URL of your Serverless Product Catalog Service"
  value       = aws_lambda_function_url.product_catalog_url.function_url
}

output "dynamodb_table_name" {
  description = "The name of the DynamoDB table"
  value       = aws_dynamodb_table.products.name
}
