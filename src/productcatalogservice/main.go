package main

import (
	"os"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/sirupsen/logrus"
)

var (
	catalogMutex *sync.Mutex
	log          *logrus.Logger
)

func init() {
	log = logrus.New()
	log.Formatter = &logrus.JSONFormatter{
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyTime:  "timestamp",
			logrus.FieldKeyLevel: "severity",
			logrus.FieldKeyMsg:   "message",
		},
		TimestampFormat: time.RFC3339Nano,
	}
	log.Out = os.Stdout
	catalogMutex = &sync.Mutex{}
}

func main() {
	svc := &productCatalog{}

	// Pre-load the catalog during Cold Start
	err := loadCatalog(&svc.products)
	if err != nil {
		log.Fatalf("could not load product catalog: %v", err)
	}

	log.Info("Starting AWS Lambda Serverless handler...")
	lambda.Start(svc.LambdaHandler)
}
