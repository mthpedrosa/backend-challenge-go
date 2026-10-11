package sqsconsumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"go.uber.org/fx"

	"github.com/mthpedrosa/backend-challenge-go/internal/app/usecase"
	"github.com/mthpedrosa/backend-challenge-go/internal/config"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/port"
	"github.com/mthpedrosa/backend-challenge-go/internal/platform/observability"
)

// Consumer is the SQS long-polling consumer.
type Consumer struct {
	cfg       config.AWSConfig
	sqs       *sqs.Client
	inbox     port.InboxRepository
	usecase   *usecase.ProcessWagerUseCase
	logger    *slog.Logger
	txManager port.TxManager

	done chan struct{}
	wg   sync.WaitGroup
}

type ConsumerParams struct {
	fx.In

	Config    config.Config
	SQS       *sqs.Client
	Inbox     port.InboxRepository
	UseCase   *usecase.ProcessWagerUseCase
	Logger    *slog.Logger
	TxManager port.TxManager
}

func NewConsumer(lc fx.Lifecycle, p ConsumerParams) *Consumer {
	c := &Consumer{
		cfg:       p.Config.AWS,
		sqs:       p.SQS,
		inbox:     p.Inbox,
		usecase:   p.UseCase,
		logger:    p.Logger.With(slog.String("component", "sqs_consumer")),
		txManager: p.TxManager,
		done:      make(chan struct{}),
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if c.cfg.InboxQueueURL == "" {
				c.logger.Warn("inbox queue URL not configured, sqs consumer disabled")
				return nil
			}
			go c.startLoop()
			c.logger.Info("sqs consumer started")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			close(c.done)
			// wait for in-flight messages to finish or deadline
			doneCh := make(chan struct{})
			go func() {
				c.wg.Wait()
				close(doneCh)
			}()

			select {
			case <-doneCh:
				c.logger.Info("sqs consumer stopped gracefully")
			case <-ctx.Done():
				c.logger.Warn("sqs consumer stopped with timeout")
			}
			return nil
		},
	})

	return c
}

func (c *Consumer) startLoop() {
	c.wg.Add(1)
	defer c.wg.Done()

	for {
		select {
		case <-c.done:
			return
		default:
		}

		out, err := c.sqs.ReceiveMessage(context.Background(), &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(c.cfg.InboxQueueURL),
			MaxNumberOfMessages: 10,
			WaitTimeSeconds:     20, // long polling
		})

		if err != nil {
			c.logger.Error("failed to receive messages", slog.Any("error", err))
			time.Sleep(2 * time.Second) // backoff on error
			continue
		}

		for _, msg := range out.Messages {
			c.wg.Add(1)
			go func(m types.Message) {
				defer c.wg.Done()
				c.handleMessage(m)
			}(msg)
		}
	}
}

type sqsEnvelope struct {
	MessageID  string          `json:"messageId"`
	Type       string          `json:"type"`
	OccurredAt string          `json:"occurredAt"`
	Data       json.RawMessage `json:"data"`
}

type sqsMessageData struct {
	ProviderID            string           `json:"providerId"`
	ExternalTransactionID string           `json:"externalTransactionId"`
	IdempotencyKey        string           `json:"idempotencyKey"`
	PlayerID              string           `json:"playerId"`
	WalletID              string           `json:"walletId"`
	RoundID               string           `json:"roundId"`
	GameID                string           `json:"gameId"`
	Kind                  string           `json:"kind"`
	Amount                string           `json:"amount"`
	Currency              string           `json:"currency"`
	Money                 *sqsMoneyPayload `json:"money"`
	ReferenceExternalID   *string          `json:"referenceExternalTransactionId,omitempty"`
}

type sqsMoneyPayload struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

func (c *Consumer) handleMessage(msg types.Message) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if msg.Body == nil {
		return
	}

	var input usecase.ProcessWagerInput
	messageID := ""
	if msg.MessageId != nil {
		messageID = *msg.MessageId
	}

	// 1. Try to unmarshal specification envelope: {"messageId": "...", "type": "...", "data": {...}}
	var env sqsEnvelope
	if err := json.Unmarshal([]byte(*msg.Body), &env); err == nil && len(env.Data) > 0 {
		if env.MessageID != "" {
			messageID = env.MessageID
		}
		var d sqsMessageData
		if err := json.Unmarshal(env.Data, &d); err == nil && (d.ProviderID != "" || d.WalletID != "") {
			amt := d.Amount
			curr := d.Currency
			if d.Money != nil {
				if d.Money.Amount != "" {
					amt = d.Money.Amount
				}
				if d.Money.Currency != "" {
					curr = d.Money.Currency
				}
			}
			input = usecase.ProcessWagerInput{
				ProviderID:            d.ProviderID,
				ExternalTransactionID: d.ExternalTransactionID,
				IdempotencyKey:        d.IdempotencyKey,
				PlayerID:              d.PlayerID,
				WalletID:              d.WalletID,
				Currency:              curr,
				Amount:                amt,
				Kind:                  d.Kind,
				RoundID:               d.RoundID,
				GameID:                d.GameID,
				ReferenceExternalID:   d.ReferenceExternalID,
			}
		}
	}

	// 2. Fallback to flat payload if input wasn't populated from envelope
	if input.ProviderID == "" && input.WalletID == "" {
		if err := json.Unmarshal([]byte(*msg.Body), &input); err != nil {
			c.logger.Error("failed to parse sqs message body", slog.Any("error", err))
			observability.Metrics.SQSMessagesReceivedTotal.WithLabelValues("malformed").Inc()
			observability.Metrics.DLQMessagesTotal.Inc()
			return
		}
	}

	// Generate deterministic hash for inbox deduplication
	payloadHash := input.GeneratePayloadHash()

	inboxMsg := port.InboxMessage{
		ConsumerName: "wager-transactions",
		MessageID:    messageID,
		PayloadHash:  payloadHash,
		ReceivedAt:   time.Now().UTC(),
	}

	err := c.txManager.RunInTx(ctx, func(txCtx context.Context) error {
		// 1. Inbox Deduplication
		inserted, existing, err := c.inbox.TryLock(txCtx, inboxMsg)
		if err != nil {
			return fmt.Errorf("inbox check failed: %w", err)
		}

		if !inserted {
			if existing.PayloadHash != payloadHash {
				return fmt.Errorf("message %s already processed with different payload (DLQ)", messageID)
			}
			observability.Metrics.InboxDuplicatedTotal.Inc()
			// Already processed (idempotency), just acknowledge
			return nil
		}

		// 2. Execute UseCase
		_, err = c.usecase.Execute(txCtx, input)
		if err != nil {
			return fmt.Errorf("usecase execution failed: %w", err)
		}

		// 3. Mark Inbox as Completed
		if err := c.inbox.MarkCompleted(txCtx, inboxMsg.ConsumerName, inboxMsg.MessageID, time.Now().UTC()); err != nil {
			return fmt.Errorf("failed to mark inbox completed: %w", err)
		}

		return nil
	})

	if err != nil {
		c.logger.Error("failed to process message", slog.String("messageId", messageID), slog.Any("error", err))
		observability.Metrics.SQSMessagesReceivedTotal.WithLabelValues("error").Inc()
		observability.Metrics.DLQMessagesTotal.Inc()
		// Leave the message so it redrives to DLQ after MaxReceiveCount
		return
	}

	// 4. Delete from queue upon successful processing
	_, err = c.sqs.DeleteMessage(context.Background(), &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(c.cfg.InboxQueueURL),
		ReceiptHandle: msg.ReceiptHandle,
	})
	if err != nil {
		c.logger.Error("failed to delete message", slog.String("messageId", messageID), slog.Any("error", err))
	} else {
		c.logger.Debug("processed sqs message successfully", slog.String("messageId", messageID))
		observability.Metrics.SQSMessagesReceivedTotal.WithLabelValues("success").Inc()
	}
}
