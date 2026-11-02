// Package svcerr builds RPC errors that are safe to return to clients.
//
// An error from this package has a public message for the client and an
// optional cause for logs only. The sanitize interceptor passes these errors
// unchanged and replaces every other server-side error with an internal error.
package svcerr

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/protobuf/proto"
)

const internalMsg = "internal error"

// publicError is the underlying error of a connect.Error built by this
// package. Error returns only the public message, so the cause never reaches
// the wire.
type publicError struct {
	msg   string
	cause error
}

func (e *publicError) Error() string { return e.msg }
func (e *publicError) Unwrap() error { return e.cause }

// New returns an error with code and a public message.
func New(code connect.Code, msg string) *connect.Error {
	return Wrap(code, msg, nil)
}

// Wrap returns an error with code and a public message. cause is kept for
// errors.Is, errors.As, and logs; the client does not see it.
func Wrap(code connect.Code, msg string, cause error) *connect.Error {
	return connect.NewError(code, &publicError{msg: msg, cause: cause})
}

// Internal returns an Internal error with a generic public message.
func Internal(cause error) *connect.Error {
	return Wrap(connect.CodeInternal, internalMsg, cause)
}

// NotFound returns a NotFound error with a ResourceInfo detail.
func NotFound(kind, id string) *connect.Error {
	err := New(connect.CodeNotFound, fmt.Sprintf("%s %q not found", kind, id))
	addDetail(err, &errdetails.ResourceInfo{ResourceType: kind, ResourceName: id})

	return err
}

// FieldViolation is one invalid field of a request.
type FieldViolation struct {
	// Field is the path of the field, for example "items[0].sku".
	Field  string
	Reason string
}

// Invalid returns an InvalidArgument error for one field.
func Invalid(field, reason string) *connect.Error {
	return InvalidFields(FieldViolation{Field: field, Reason: reason})
}

// InvalidFields returns an InvalidArgument error with a BadRequest detail
// that lists every violation. The message names the first one.
func InvalidFields(violations ...FieldViolation) *connect.Error {
	msg := "invalid argument"

	if len(violations) > 0 {
		msg = fmt.Sprintf("invalid argument: %s: %s", violations[0].Field, violations[0].Reason)
		if n := len(violations) - 1; n > 0 {
			msg += fmt.Sprintf(" (and %d more)", n)
		}
	}

	err := New(connect.CodeInvalidArgument, msg)

	br := &errdetails.BadRequest{}
	for _, v := range violations {
		br.FieldViolations = append(br.FieldViolations,
			&errdetails.BadRequest_FieldViolation{Field: v.Field, Description: v.Reason})
	}

	addDetail(err, br)

	return err
}

// Sanitize returns the error to send to the client for err, and whether err
// was replaced. The caller logs err when it was replaced.
//
//   - An error built by this package is returned unchanged.
//   - A context deadline or cancellation becomes DeadlineExceeded or Canceled.
//   - A connect.Error with a client-side code (for example NotFound) is
//     returned unchanged.
//   - Every other error becomes Internal. This includes errors received from
//     other servers, so that their messages do not pass through.
func Sanitize(err error) (*connect.Error, bool) {
	if err == nil {
		return nil, false
	}

	var ce *connect.Error
	if errors.As(err, &ce) {
		// Only a direct publicError is safe: a deeper one could sit under a
		// message that is not public.
		if _, ok := ce.Unwrap().(*publicError); ok { //nolint:errorlint // see above
			return ce, false
		}
	}

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return Wrap(connect.CodeDeadlineExceeded, "deadline exceeded", err), false
	case errors.Is(err, context.Canceled):
		return Wrap(connect.CodeCanceled, "canceled", err), false
	case ce != nil && !connect.IsWireError(ce) && !serverFault(ce.Code()):
		return ce, false
	default:
		return Internal(err), true
	}
}

func serverFault(c connect.Code) bool {
	switch c { //nolint:exhaustive // every other code is a client-side condition
	case connect.CodeUnknown, connect.CodeInternal, connect.CodeDataLoss:
		return true
	default:
		return false
	}
}

func addDetail(err *connect.Error, msg proto.Message) {
	// NewErrorDetail fails only when msg cannot be marshaled, which does not
	// happen for the well-known detail types used here.
	if d, derr := connect.NewErrorDetail(msg); derr == nil {
		err.AddDetail(d)
	}
}
