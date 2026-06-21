#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}Microservices E-Commerce Deployment${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

# Check prerequisites
echo -e "${YELLOW}Checking prerequisites...${NC}"

# Check Go
if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: Go is not installed${NC}"
    echo "Install from: https://golang.org/dl/"
    exit 1
fi
echo -e "${GREEN}✓ Go installed: $(go version)${NC}"

# Check AWS CLI
if ! command -v aws &> /dev/null; then
    echo -e "${RED}Error: AWS CLI is not installed${NC}"
    echo "Run: brew install awscli"
    exit 1
fi
echo -e "${GREEN}✓ AWS CLI installed${NC}"

# Check Terraform
if ! command -v terraform &> /dev/null; then
    echo -e "${RED}Error: Terraform is not installed${NC}"
    echo "Run: brew install terraform"
    exit 1
fi
echo -e "${GREEN}✓ Terraform installed${NC}"

# Check AWS credentials
if ! aws sts get-caller-identity &> /dev/null; then
    echo -e "${RED}Error: AWS credentials not configured${NC}"
    echo "Run: aws configure"
    exit 1
fi
echo -e "${GREEN}✓ AWS credentials configured${NC}"
echo ""

# Get AWS account info
AWS_ACCOUNT=$(aws sts get-caller-identity --query Account --output text)
AWS_REGION=$(aws configure get region)
echo -e "${GREEN}AWS Account: ${AWS_ACCOUNT}${NC}"
echo -e "${GREEN}AWS Region: ${AWS_REGION}${NC}"
echo ""

# Check for Stripe keys
echo -e "${YELLOW}Checking Stripe configuration...${NC}"
if ! aws ssm get-parameter --name "/paymentservice/stripe_secret_key" --with-decryption &> /dev/null; then
    echo -e "${YELLOW}⚠ Stripe secret key not found in SSM${NC}"
    echo -e "${YELLOW}Payment service will not work without Stripe keys${NC}"
    echo ""
    read -p "Do you want to set up Stripe keys now? (y/n): " setup_stripe
    if [ "$setup_stripe" = "y" ]; then
        read -p "Enter your Stripe Secret Key (sk_test_...): " stripe_key
        read -p "Enter your Stripe Webhook Secret (whsec_...): " webhook_secret
        
        aws ssm put-parameter \
            --name "/paymentservice/stripe_secret_key" \
            --value "$stripe_key" \
            --type "SecureString" \
            --overwrite
        
        aws ssm put-parameter \
            --name "/paymentservice/stripe_webhook_secret" \
            --value "$webhook_secret" \
            --type "SecureString" \
            --overwrite
        
        echo -e "${GREEN}✓ Stripe keys stored in AWS SSM${NC}"
    else
        echo -e "${YELLOW}Skipping payment service deployment${NC}"
    fi
else
    echo -e "${GREEN}✓ Stripe keys found in SSM${NC}"
fi
echo ""

# Deployment order (dependencies matter)
# Skipping paymentservice temporarily
SERVICES=("productcatalogservice" "cartservice" "checkoutservice")

for service in "${SERVICES[@]}"; do
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}Deploying ${service}...${NC}"
    echo -e "${GREEN}========================================${NC}"
    
    cd "src/${service}"
    
    # Build the Go binary
    echo -e "${YELLOW}Building ${service}...${NC}"
    
    if [ "$service" = "productcatalogservice" ]; then
        # Product catalog uses ARM64 and needs all .go files
        GOOS=linux GOARCH=arm64 go build -tags lambda.norpc -o bootstrap .
        echo -e "${GREEN}✓ Built ${service} (ARM64)${NC}"
    else
        # Other services use AMD64 - build all .go files in directory
        GOOS=linux GOARCH=amd64 go build -tags lambda.norpc -o bootstrap .
        zip deployment.zip bootstrap
        echo -e "${GREEN}✓ Built ${service} (AMD64)${NC}"
    fi
    
    # Build additional Lambda functions for specific services
    if [ "$service" = "checkoutservice" ]; then
        echo -e "${YELLOW}Building payment consumer...${NC}"
        cd paymentconsumer
        GOOS=linux GOARCH=amd64 go build -tags lambda.norpc -o bootstrap .
        zip ../paymentconsumer.zip bootstrap
        cd ..
        echo -e "${GREEN}✓ Built payment consumer${NC}"
    fi
    
    # Deploy with Terraform
    echo -e "${YELLOW}Deploying infrastructure...${NC}"
    terraform init -upgrade
    
    if [ "$service" = "checkoutservice" ]; then
        # Checkout service needs references to other services
        # Payment service is disabled, so we leave payment_function_name empty
        terraform apply -auto-approve \
            -var="cart_function_name=cartservice" \
            -var="catalog_function_name=productcatalogservice" \
            -var="payment_function_name="
    else
        terraform apply -auto-approve
    fi
    
    echo -e "${GREEN}✓ ${service} deployed successfully!${NC}"
    echo ""
    
    # Capture outputs
    if [ "$service" = "productcatalogservice" ]; then
        CATALOG_URL=$(terraform output -raw lambda_endpoint 2>/dev/null || echo "N/A")
        echo -e "${GREEN}Product Catalog URL: ${CATALOG_URL}${NC}"
    fi
    
    if [ "$service" = "checkoutservice" ]; then
        CHECKOUT_URL=$(terraform output -raw checkout_function_url 2>/dev/null || echo "N/A")
        echo -e "${GREEN}Checkout URL: ${CHECKOUT_URL}${NC}"
    fi
    
    if [ "$service" = "paymentservice" ]; then
        WEBHOOK_URL=$(terraform output -raw webhook_url 2>/dev/null || echo "N/A")
        echo -e "${GREEN}Stripe Webhook URL: ${WEBHOOK_URL}${NC}"
        echo -e "${YELLOW}⚠ Configure this URL in your Stripe Dashboard!${NC}"
    fi
    
    cd ../..
    echo ""
done

# Load sample products
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}Loading Sample Products${NC}"
echo -e "${GREEN}========================================${NC}"
./scripts/load-products.sh

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}Deployment Complete! 🎉${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo -e "${YELLOW}Next Steps:${NC}"
echo "1. Configure Stripe webhook URL in your Stripe Dashboard"
echo "2. Test the APIs using the provided URLs"
echo "3. Check CloudWatch Logs for any errors"
echo ""
echo -e "${YELLOW}To destroy all resources:${NC}"
echo "  ./destroy.sh"
echo ""
