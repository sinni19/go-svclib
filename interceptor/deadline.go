package interceptor

import (
	"context"
	"time"

	"connectrpc.com/connect"
)

// Deadline gives each unary call a deadline: def when the client sets none,
// and at most maxTimeout when the client sets a longer one.
//
// Streams pass through unchanged, so that long-lived streams are possible.
// The client deadline and the App shutdown still bound them.
func Deadline(def, maxTimeout time.Duration) connect.Interceptor {
	return handlerOnly{
		unary: func(next connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				timeout := def
				if d, ok := ctx.Deadline(); ok {
					timeout = min(time.Until(d), maxTimeout)
				}

				ctx, cancel := context.WithTimeout(ctx, timeout)
				defer cancel()

				return next(ctx, req)
			}
		},
		stream: func(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc { return next },
	}
}
