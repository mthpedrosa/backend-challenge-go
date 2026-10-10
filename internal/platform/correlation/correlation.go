// Package correlation carries the correlation id across layers through
// context.Context, without coupling them to HTTP or SQS.
package correlation

import "context"

// Header is the HTTP header used to receive and return the correlation id.
const Header = "X-Correlation-ID"

type ctxKey struct{}

// WithID returns a copy of ctx carrying the given correlation id.
func WithID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext returns the correlation id stored in ctx, if any.
func FromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(ctxKey{}).(string)
	return id, ok && id != ""
}
