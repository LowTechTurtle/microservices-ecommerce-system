#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${RED}========================================${NC}"
echo -e "${RED}Destroying All AWS Resources${NC}"
echo -e "${RED}========================================${NC}"
echo ""

read -p "Are you sure you want to destroy all resources? (yes/no): " confirm
if [ "$confirm" != "yes" ]; then
    echo "Destruction cancelled."
    exit 0
fi

# Destroy in reverse order
SERVICES=("checkoutservice" "paymentservice" "cartservice" "productcatalogservice")

for service in "${SERVICES[@]}"; do
    echo -e "${YELLOW}Destroying ${service}...${NC}"
    cd "src/${service}"
    
    if [ -d ".terraform" ]; then
        terraform destroy -auto-approve
        echo -e "${GREEN}✓ ${service} destroyed${NC}"
    else
        echo -e "${YELLOW}⚠ ${service} not deployed${NC}"
    fi
    
    cd ../..
done

echo ""
echo -e "${GREEN}All resources destroyed!${NC}"
