#!/bin/bash
set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}Testing Deployed Services${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

# Get service URLs
cd src/productcatalogservice
CATALOG_URL=$(terraform output -raw lambda_endpoint 2>/dev/null || echo "")
cd ../..

cd src/checkoutservice
CHECKOUT_URL=$(terraform output -raw checkout_function_url 2>/dev/null || echo "")
cd ../..

if [ -z "$CATALOG_URL" ] || [ -z "$CHECKOUT_URL" ]; then
    echo -e "${RED}Error: Services not deployed${NC}"
    exit 1
fi

echo -e "${GREEN}Service URLs:${NC}"
echo -e "Catalog: ${CATALOG_URL}"
echo -e "Checkout: ${CHECKOUT_URL}"
echo ""

# Test 1: List Products
echo -e "${YELLOW}Test 1: Listing products...${NC}"
curl -s -X POST "${CATALOG_URL}" \
    -H "Content-Type: application/json" \
    -d '{"action": "ListProducts"}' | jq '.'
echo ""

# Test 2: Get a specific product
echo -e "${YELLOW}Test 2: Getting product details...${NC}"
curl -s -X POST "${CATALOG_URL}" \
    -H "Content-Type: application/json" \
    -d '{"action": "GetProduct", "get_product_req": {"id": "ABCDEFGHI1"}}' | jq '.'
echo ""

# Test 3: Add item to cart (via checkout service)
echo -e "${YELLOW}Test 3: Adding item to cart...${NC}"
TEST_USER="test-user-$(date +%s)"
curl -s -X POST "${CHECKOUT_URL}" \
    -H "Content-Type: application/json" \
    -d "{
        \"action\": \"AddToCart\",
        \"userId\": \"${TEST_USER}\",
        \"productId\": \"ABCDEFGHI1\",
        \"quantity\": 2
    }" | jq '.'
echo ""

# Test 4: Get cart
echo -e "${YELLOW}Test 4: Getting cart contents...${NC}"
curl -s -X POST "${CHECKOUT_URL}" \
    -H "Content-Type: application/json" \
    -d "{
        \"action\": \"GetCart\",
        \"userId\": \"${TEST_USER}\"
    }" | jq '.'
echo ""

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}Testing Complete!${NC}"
echo -e "${GREEN}========================================${NC}"
