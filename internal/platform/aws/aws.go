package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"go.uber.org/fx"

	"github.com/mthpedrosa/backend-challenge-go/internal/config"
)

// Module provides AWS clients.
var Module = fx.Module("aws",
	fx.Provide(
		NewConfig,
		NewSQSClient,
	),
)

// NewConfig initializes the AWS configuration.
func NewConfig(cfg config.Config) (aws.Config, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.AWS.Region),
	}

	accessKey := cfg.AWS.AccessKeyID
	secretKey := cfg.AWS.SecretAccessKey
	if cfg.AWS.EndpointURL != "" && (accessKey == "" || secretKey == "") {
		accessKey = "test"
		secretKey = "test"
	}

	if accessKey != "" && secretKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return aws.Config{}, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return awsCfg, nil
}

// NewSQSClient creates a new SQS client.
func NewSQSClient(cfg config.Config, awsCfg aws.Config) *sqs.Client {
	if cfg.AWS.EndpointURL != "" {
		// Use custom endpoint resolver for LocalStack/MiniStack
		return sqs.NewFromConfig(awsCfg, func(o *sqs.Options) {
			o.BaseEndpoint = aws.String(cfg.AWS.EndpointURL)
		})
	}
	return sqs.NewFromConfig(awsCfg)
}
