#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}Microservices Deployment (No Payment)${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

# Check prerequisites
echo -e "${YELLOW}Checking prerequisites...${NC}"

# Check Go
if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: Go is not installed${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Go installed: $(go version)${NC}"

# Check AWS CLI
if ! command -v aws &> /dev/null; then
    echo -e "${RED}Error: AWS CLI is not installed${NC}"
    exit 1
fi
echo -e "${GREEN}✓ AWS CLI installed${NC}"

# Check Terraform
if ! command -v terraform &> /dev/null; then
    echo -e "${RED}Error: Terraform is not installed${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Terraform installed${NC}"

# Check AWS credentials
if ! aws sts get-caller-identity &> /dev/null; then
    echo -e "${RED}Error: AWS credentials not configured${NC}"
    exit 1
fi
echo -e "${GREEN}✓ AWS credentials configured${NC}"

AWS_ACCOUNT=$(aws sts get-caller-identity --query Account --output text)
AWS_REGION=$(aws configure get region)
echo -e "${GREEN}AWS Account: ${AWS_ACCOUNT}${NC}"
echo -e "${GREEN}AWS Region: ${AWS_REGION}${NC}"
echo ""

# Deployment order: catalog → cart → checkout
SERVICES=("productcatalogservice" "cartservice" "checkoutservice")

for service in "${SERVICES[@]}"; do
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}Deploying ${service}...${NC}"
    echo -e "${GREEN}========================================${NC}"
    
    cd "src/${service}"
    
    # Clean up old builds
    rm -f bootstrap deployment.zip paymentconsumer.zip
    
    # Build
    echo -e "${YELLOW}Building ${service}...${NC}"
    
    if [ "$service" = "productcatalogservice" ]; then
        # Build all Go files together for ARM64
        GOOS=linux GOARCH=arm64 go build -tags lambda.norpc -o bootstrap .
        echo -e "${GREEN}✓ Built ${service} (ARM64)${NC}"
    else
        # Build all Go files for AMD64
        GOOS=linux GOARCH=amd64 go build -tags lambda.norpc -o bootstrap .
        zip deployment.zip bootstrap
        echo -e "${GREEN}✓ Built ${service} (AMD64)${NC}"
    fi
    
    # Build payment consumer for checkout service
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
        # Checkout needs references but no payment service
        terraform apply -auto-approve \
            -var="cart_function_name=cartservice" \
            -var="catalog_function_name=productcatalogservice" \
            -var="payment_function_name="
    else
        terraform apply -auto-approve
    fi
    
    echo -e "${GREEN}✓ ${service} deployed successfully!${NC}"
    
    # Show outputs
    if [ "$service" = "productcatalogservice" ]; then
        CATALOG_URL=$(terraform output -raw lambda_endpoint 2>/dev/null || echo "N/A")
        echo -e "${GREEN}Product Catalog URL: ${CATALOG_URL}${NC}"
    fi
    
    if [ "$service" = "checkoutservice" ]; then
        CHECKOUT_URL=$(terraform output -raw checkout_function_url 2>/dev/null || echo "N/A")
        echo -e "${GREEN}Checkout URL: ${CHECKOUT_URL}${NC}"
    fi
    
    cd ../..
    echo ""
done

# Load sample products
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}Loading Sample Products${NC}"
echo -e "${GREEN}========================================${NC}"

cd src/productcatalogservice

if command -v jq &> /dev/null; then
    echo -e "${YELLOW}Loading products into DynamoDB...${NC}"
    jq -c '.products[]' products.json | while read product; do
        id=$(echo $product | jq -r '.id')
        name=$(echo $product | jq -r '.name')
        
        echo -e "${YELLOW}Inserting: ${name} (${id})${NC}"
        
        aws dynamodb put-item \
            --table-name products \
            --item "$(echo $product | jq -c '{
                id: {S: .id},
                name: {S: .name},
                description: {S: .description},
                picture: {S: .picture},
                price_usd_currency_code: {S: .priceUsd.currencyCode},
                price_usd_units: {N: (.priceUsd.units | tostring)},
                price_usd_nanos: {N: (.priceUsd.nanos | tostring)},
                categories: {SS: .categories}
            }')" 2>/dev/null || echo -e "${YELLOW}  (already exists or error)${NC}"
    done
    echo -e "${GREEN}✓ Products loaded${NC}"
else
    echo -e "${YELLOW}⚠ jq not installed, skipping product loading${NC}"
    echo -e "${YELLOW}Install jq: brew install jq${NC}"
fi

cd ../..

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}Deployment Complete! 🎉${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo -e "${YELLOW}Services Deployed:${NC}"
echo "  ✅ Product Catalog Service"
echo "  ✅ Cart Service"
echo "  ✅ Checkout Service"
echo ""
echo -e "${YELLOW}⚠ Payment Service: SKIPPED${NC}"
echo ""
echo -e "${YELLOW}To test your deployment:${NC}"
echo "  ./scripts/test-services.sh"
echo ""
echo -e "${YELLOW}To destroy all resources:${NC}"
echo "  ./destroy.sh"
echo ""
