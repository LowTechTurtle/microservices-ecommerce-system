#!/bin/bash
set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${YELLOW}Loading sample products into DynamoDB...${NC}"

# Read products from JSON and insert into DynamoDB
cd src/productcatalogservice

# Check if products.json exists
if [ ! -f "products.json" ]; then
    echo "Error: products.json not found"
    exit 1
fi

# Parse JSON and insert each product
jq -c '.products[]' products.json | while read product; do
    id=$(echo $product | jq -r '.id')
    name=$(echo $product | jq -r '.name')
    
    echo -e "${YELLOW}Inserting product: ${name} (${id})${NC}"
    
    aws dynamodb put-item \
        --table-name products \
        --item "$(echo $product | jq -c '{
            id: {S: .id},
            name: {S: .name},
            description: {S: .description},
            picture: {S: .picture},
            priceUsd: {
                M: {
                    currencyCode: {S: .priceUsd.currencyCode},
                    units: {N: (.priceUsd.units | tostring)},
                    nanos: {N: (.priceUsd.nanos | tostring)}
                }
            },
            categories: {SS: .categories}
        }')"
done

echo -e "${GREEN}✓ Sample products loaded successfully!${NC}"
cd ../..
