package interceptor

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"

	"connectrpc.com/connect"

	"github.com/sinni19/go-svclib/svcerr"
)

// Recover turns a handler panic into an Internal error. The stack goes to the
// log only. A panic with http.ErrAbortHandler is logged the same way.
func Recover(logger *slog.Logger) connect.Interceptor {
	return handlerOnly{
		unary: func(next connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (res connect.AnyResponse, err error) {
				defer recoverTo(ctx, logger, req.Spec().Procedure, &err)

				return next(ctx, req)
			}
		},
		stream: func(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
			return func(ctx context.Context, conn connect.StreamingHandlerConn) (err error) {
				defer recoverTo(ctx, logger, conn.Spec().Procedure, &err)

				return next(ctx, conn)
			}
		},
	}
}

func recoverTo(ctx context.Context, logger *slog.Logger, procedure string, err *error) {
	r := recover()
	if r == nil {
		return
	}

	logger.ErrorContext(ctx, "rpc handler panicked",
		"procedure", procedure, "panic", r, "stack", string(debug.Stack()))

	*err = svcerr.Internal(fmt.Errorf("panic: %v", r))
}
