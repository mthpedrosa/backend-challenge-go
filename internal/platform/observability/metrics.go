package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// AppMetrics holds the prometheus metrics collectors for the application.
type AppMetrics struct {
	// Business & domain counters
	TransactionProcessedTotal *prometheus.CounterVec
	TransactionRejectedTotal  *prometheus.CounterVec
	WalletDuplicatedTotal     prometheus.Counter
	IdempotencyConflictTotal  prometheus.Counter
	ReconciliationMismatch    prometheus.Counter

	// SQS & Inbox metrics
	SQSMessagesReceivedTotal *prometheus.CounterVec
	InboxDuplicatedTotal     prometheus.Counter
	DLQMessagesTotal         prometheus.Counter

	// Worker metrics
	OutboxPublishSuccessTotal prometheus.Counter
	OutboxPublishErrorTotal   prometheus.Counter
	OutboxPendingLag          prometheus.Gauge
	PendingReferenceRetries   prometheus.Counter

	// HTTP metrics
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
}

var Metrics = AppMetrics{
	TransactionProcessedTotal: promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "wallet_transaction_processed_total",
		Help: "Total number of processed wager transactions",
	}, []string{"kind", "origin"}),

	TransactionRejectedTotal: promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "wallet_transaction_rejected_total",
		Help: "Total number of rejected wager transactions",
	}, []string{"kind", "failure_code"}),

	WalletDuplicatedTotal: promauto.NewCounter(prometheus.CounterOpts{
		Name: "wallet_duplicated_total",
		Help: "Total number of wallet creation conflicts",
	}),

	IdempotencyConflictTotal: promauto.NewCounter(prometheus.CounterOpts{
		Name: "wallet_idempotency_conflict_total",
		Help: "Total number of idempotency conflicts (same key, different payload)",
	}),

	ReconciliationMismatch: promauto.NewCounter(prometheus.CounterOpts{
		Name: "wallet_reconciliation_mismatch_total",
		Help: "Total number of divergent balances detected during reconciliation",
	}),

	SQSMessagesReceivedTotal: promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "wallet_sqs_messages_received_total",
		Help: "Total SQS messages received by outcome",
	}, []string{"outcome"}), // e.g. success, error, malformed

	InboxDuplicatedTotal: promauto.NewCounter(prometheus.CounterOpts{
		Name: "wallet_inbox_duplicated_total",
		Help: "Total number of duplicated SQS messages skipped via Inbox",
	}),

	DLQMessagesTotal: promauto.NewCounter(prometheus.CounterOpts{
		Name: "wallet_dlq_messages_total",
		Help: "Total number of messages sent to DLQ (simulated by failure limits)",
	}),

	OutboxPublishSuccessTotal: promauto.NewCounter(prometheus.CounterOpts{
		Name: "wallet_outbox_publish_success_total",
		Help: "Total outbox events successfully published to SQS",
	}),

	OutboxPublishErrorTotal: promauto.NewCounter(prometheus.CounterOpts{
		Name: "wallet_outbox_publish_error_total",
		Help: "Total outbox publish attempts that failed",
	}),

	OutboxPendingLag: promauto.NewGauge(prometheus.GaugeOpts{
		Name: "wallet_outbox_pending_lag",
		Help: "Current estimated lag of unpublished events in the outbox",
	}),

	PendingReferenceRetries: promauto.NewCounter(prometheus.CounterOpts{
		Name: "wallet_pending_reference_retries_total",
		Help: "Total retries triggered by the Pending Reference worker",
	}),

	HTTPRequestsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "wallet_http_requests_total",
		Help: "Total HTTP requests processed",
	}, []string{"method", "path", "status"}),

	HTTPRequestDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "wallet_http_request_duration_seconds",
		Help:    "HTTP request latency distributions",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"}),
}
