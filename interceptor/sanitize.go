package interceptor

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"

	"github.com/sinni19/go-svclib/svcerr"
)

// Sanitize replaces every handler error that is not safe for clients with an
// Internal error, and logs the original. See svcerr.Sanitize for the rules.
func Sanitize(logger *slog.Logger) connect.Interceptor {
	return handlerOnly{
		unary: func(next connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				res, err := next(ctx, req)
				if err != nil {
					return nil, sanitize(ctx, logger, req.Spec().Procedure, err)
				}

				return res, nil
			}
		},
		stream: func(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
			return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
				if err := next(ctx, conn); err != nil {
					return sanitize(ctx, logger, conn.Spec().Procedure, err)
				}

				return nil
			}
		},
	}
}

func sanitize(ctx context.Context, logger *slog.Logger, procedure string, err error) error {
	safe, replaced := svcerr.Sanitize(err)
	if replaced {
		logger.ErrorContext(ctx, "rpc failed with internal error", "procedure", procedure, "error", err)
	}

	return safe
}
