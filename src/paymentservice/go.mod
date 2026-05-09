module paymentservice

go 1.26.2

require (
	github.com/aws/aws-lambda-go v1.54.0
	github.com/aws/aws-sdk-go-v2 v1.41.5
	github.com/aws/aws-sdk-go-v2/config v1.32.14
	github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue v1.20.37
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.57.1
	github.com/aws/aws-sdk-go-v2/service/eventbridge v1.38.4
	github.com/google/uuid v1.6.0
	github.com/sirupsen/logrus v1.9.3
	github.com/stripe/stripe-go/v76 v76.25.0
)
