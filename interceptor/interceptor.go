// Package interceptor holds the server interceptors that go-svclib installs on
// every RPC handler, for unary and streaming calls.
//
// The App installs them in a fixed order (see docs/plan.md, section 4.2).
// Each interceptor wraps handlers only; client calls pass through unchanged.
package interceptor

import (
	"context"

	"connectrpc.com/connect"
)

// handlerOnly is a connect.Interceptor built from handler-side wrappers.
type handlerOnly struct {
	unary  func(connect.UnaryFunc) connect.UnaryFunc
	stream func(connect.StreamingHandlerFunc) connect.StreamingHandlerFunc
}

func (h handlerOnly) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	wrapped := h.unary(next)

	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if req.Spec().IsClient {
			return next(ctx, req)
		}

		return wrapped(ctx, req)
	}
}

func (handlerOnly) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (h handlerOnly) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return h.stream(next)
}
