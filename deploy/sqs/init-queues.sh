#!/bin/sh

echo "Initializing SQS queues in MiniStack..."

awslocal sqs create-queue --queue-name wager-transactions-dlq.fifo --attributes FifoQueue=true,ContentBasedDeduplication=false
awslocal sqs create-queue --queue-name wager-transactions.fifo --attributes FifoQueue=true,ContentBasedDeduplication=false,RedrivePolicy="\"{\\\"deadLetterTargetArn\\\":\\\"arn:aws:sqs:us-east-1:000000000000:wager-transactions-dlq.fifo\\\",\\\"maxReceiveCount\\\":\\\"5\\\"}\""

awslocal sqs create-queue --queue-name outbox.fifo --attributes FifoQueue=true,ContentBasedDeduplication=false

echo "SQS queues initialized successfully."
