package interceptor

import (
	"context"
	"errors"
	"fmt"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"github.com/sinni19/go-svclib/svcerr"
)

// Validate checks each request message, also each message of a stream,
// against its protovalidate rules. A rule violation is InvalidArgument with a
// BadRequest detail. A message that is not a proto.Message passes unchecked.
func Validate(v protovalidate.Validator) connect.Interceptor {
	return handlerOnly{
		unary: func(next connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				if err := check(v, req.Any()); err != nil {
					return nil, err
				}

				return next(ctx, req)
			}
		},
		stream: func(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
			return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
				return next(ctx, validatingConn{StreamingHandlerConn: conn, v: v})
			}
		},
	}
}

type validatingConn struct {
	connect.StreamingHandlerConn

	v protovalidate.Validator
}

func (c validatingConn) Receive(msg any) error {
	if err := c.StreamingHandlerConn.Receive(msg); err != nil {
		return err //nolint:wrapcheck // handlers compare io.EOF with ==
	}

	return check(c.v, msg)
}

func check(v protovalidate.Validator, msg any) error {
	m, ok := msg.(proto.Message)
	if !ok {
		return nil
	}

	err := v.Validate(m)
	if err == nil {
		return nil
	}

	var ve *protovalidate.ValidationError
	if !errors.As(err, &ve) {
		// A compilation or runtime error is a broken rule, not a bad request.
		return svcerr.Internal(fmt.Errorf("validate %s: %w", m.ProtoReflect().Descriptor().FullName(), err))
	}

	violations := make([]svcerr.FieldViolation, 0, len(ve.Violations))
	for _, vi := range ve.Violations {
		violations = append(violations, svcerr.FieldViolation{
			Field:  protovalidate.FieldPathString(vi.Proto.GetField()),
			Reason: vi.Proto.GetMessage(),
		})
	}

	return svcerr.InvalidFields(violations...)
}
