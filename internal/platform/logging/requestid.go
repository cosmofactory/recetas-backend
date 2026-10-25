package logging

import "context"

type requestIDContextKey struct{}

// WithRequestID returns ctx carrying id; every line logged with it gets
// request_id.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDContextKey{}, id)
}

// RequestIDFrom returns the request ID in ctx, or "" without one.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDContextKey{}).(string)
	return id
}
