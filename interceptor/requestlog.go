package interceptor

import (
	"context"
	"log/slog"
	"time"

	"connectrpc.com/connect"
)

// RequestLog writes one log line for each finished call, with the procedure,
// protocol, result code, and duration. It runs outside Sanitize, so the code
// is the one that the client receives. A panic is logged as internal and
// passes on to Recover.
func RequestLog(logger *slog.Logger) connect.Interceptor {
	return handlerOnly{
		unary: func(next connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				start := time.Now()
				defer logPanic(ctx, logger, req.Spec(), req.Peer(), start)

				res, err := next(ctx, req)
				logCall(ctx, logger, req.Spec(), req.Peer(), start, err)

				return res, err
			}
		},
		stream: func(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
			return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
				start := time.Now()
				defer logPanic(ctx, logger, conn.Spec(), conn.Peer(), start)

				err := next(ctx, conn)
				logCall(ctx, logger, conn.Spec(), conn.Peer(), start, err)

				return err
			}
		},
	}
}

func logCall(ctx context.Context, logger *slog.Logger, spec connect.Spec, peer connect.Peer, start time.Time, err error) {
	code := "ok"
	if err != nil {
		code = connect.CodeOf(err).String()
	}

	logger.LogAttrs(ctx, slog.LevelInfo, "rpc finished",
		slog.String("procedure", spec.Procedure),
		slog.String("protocol", peer.Protocol),
		slog.String("code", code),
		slog.Duration("duration", time.Since(start)),
	)
}

// logPanic logs a call that panics, then continues the panic. The stack is
// not unwound yet, so Recover still logs the frames of the original panic.
func logPanic(ctx context.Context, logger *slog.Logger, spec connect.Spec, peer connect.Peer, start time.Time) {
	if r := recover(); r != nil {
		logCall(ctx, logger, spec, peer, start, connect.NewError(connect.CodeInternal, nil))
		panic(r)
	}
}
