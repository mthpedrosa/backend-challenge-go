package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/google/uuid"
	"go.uber.org/fx"

	"github.com/mthpedrosa/backend-challenge-go/internal/config"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/event"
	"github.com/mthpedrosa/backend-challenge-go/internal/platform/observability"
)

// OutboxPublisher is a background worker that polls the outbox table and publishes events to SQS.
type OutboxPublisher struct {
	cfg    config.AWSConfig
	repo   event.OutboxRepository
	sqs    *sqs.Client
	logger *slog.Logger

	ticker *time.Ticker
	done   chan struct{}
}

// OutboxPublisherParams holds dependencies for the publisher.
type OutboxPublisherParams struct {
	fx.In

	Config config.Config
	Repo   event.OutboxRepository
	SQS    *sqs.Client
	Logger *slog.Logger
}

// NewOutboxPublisher creates and starts the publisher worker.
func NewOutboxPublisher(lc fx.Lifecycle, p OutboxPublisherParams) *OutboxPublisher {
	pub := &OutboxPublisher{
		cfg:    p.Config.AWS,
		repo:   p.Repo,
		sqs:    p.SQS,
		logger: p.Logger.With(slog.String("component", "outbox_publisher")),
		done:   make(chan struct{}),
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if pub.cfg.OutboxQueueURL == "" {
				pub.logger.Warn("outbox queue URL not configured, publisher disabled")
				return nil
			}
			pub.ticker = time.NewTicker(2 * time.Second)
			go pub.loop()
			pub.logger.Info("outbox publisher started")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if pub.ticker != nil {
				pub.ticker.Stop()
				close(pub.done)
				pub.logger.Info("outbox publisher stopped")
			}
			return nil
		},
	})

	return pub
}

func (p *OutboxPublisher) loop() {
	for {
		select {
		case <-p.done:
			return
		case <-p.ticker.C:
			p.processBatch()
		}
	}
}

func (p *OutboxPublisher) processBatch() {
	// Lease up to 10 events at a time
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	events, err := p.repo.LeaseUnpublished(ctx, 10)
	if err != nil {
		p.logger.Error("failed to lease outbox events", slog.Any("error", err))
		return
	}

	if len(events) == 0 {
		return
	}

	for _, e := range events {
		if err := p.publishEvent(ctx, e); err != nil {
			p.logger.Error("failed to publish event",
				slog.String("eventId", e.EventID),
				slog.Any("error", err),
			)
			observability.Metrics.OutboxPublishErrorTotal.Inc()
			continue
		}

		if err := p.repo.MarkPublished(ctx, e.EventID); err != nil {
			p.logger.Error("failed to mark event as published",
				slog.String("eventId", e.EventID),
				slog.Any("error", err),
			)
			observability.Metrics.OutboxPublishErrorTotal.Inc()
			// we don't abort, it will be retried (at-least-once delivery semantics)
		} else {
			p.logger.Debug("published event successfully", slog.String("eventId", e.EventID))
			observability.Metrics.OutboxPublishSuccessTotal.Inc()
		}
	}
}

func (p *OutboxPublisher) publishEvent(ctx context.Context, e event.Envelope) error {
	// e.Data is json.RawMessage because we scanned it as such in LeaseUnpublished
	payload, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("failed to marshal envelope: %w", err)
	}

	// SQS FIFO queues require MessageGroupId and MessageDeduplicationId.
	// MessageGroupId = aggregate_id to ensure events for the same entity are ordered.
	// MessageDeduplicationId = event_id to ensure we don't publish the exact same event twice.

	msgGrpID := e.AggregateID
	if msgGrpID == "" {
		msgGrpID = "default"
	}

	msgDedupID := e.EventID
	if msgDedupID == "" {
		msgDedupID = uuid.NewString()
	}

	_, err = p.sqs.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:               aws.String(p.cfg.OutboxQueueURL),
		MessageBody:            aws.String(string(payload)),
		MessageGroupId:         aws.String(msgGrpID),
		MessageDeduplicationId: aws.String(msgDedupID),
	})

	return err
}
