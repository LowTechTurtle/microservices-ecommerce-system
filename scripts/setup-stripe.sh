#!/bin/bash
set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}Stripe Configuration Setup${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

echo -e "${YELLOW}You need a Stripe account to use the payment service.${NC}"
echo -e "${YELLOW}Get your API keys from: https://dashboard.stripe.com/test/apikeys${NC}"
echo ""

read -p "Enter your Stripe Secret Key (sk_test_...): " stripe_key
read -p "Enter your Stripe Webhook Secret (whsec_...) [optional]: " webhook_secret

if [ -z "$stripe_key" ]; then
    echo -e "${RED}Error: Stripe secret key is required${NC}"
    exit 1
fi

# Store in AWS SSM Parameter Store
echo -e "${YELLOW}Storing keys in AWS SSM Parameter Store...${NC}"

aws ssm put-parameter \
    --name "/paymentservice/stripe_secret_key" \
    --value "$stripe_key" \
    --type "SecureString" \
    --overwrite

if [ -n "$webhook_secret" ]; then
    aws ssm put-parameter \
        --name "/paymentservice/stripe_webhook_secret" \
        --value "$webhook_secret" \
        --type "SecureString" \
        --overwrite
else
    # Use a placeholder if not provided
    aws ssm put-parameter \
        --name "/paymentservice/stripe_webhook_secret" \
        --value "placeholder_will_update_after_deployment" \
        --type "SecureString" \
        --overwrite
fi

echo -e "${GREEN}✓ Stripe keys stored successfully!${NC}"
echo ""
echo -e "${YELLOW}Note: After deploying the payment service, you'll get a webhook URL.${NC}"
echo -e "${YELLOW}Add that URL to your Stripe Dashboard under Developers → Webhooks${NC}"
