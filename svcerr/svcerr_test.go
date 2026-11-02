package svcerr

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
)

var errDB = errors.New("pq: relation \"orders\" does not exist")

func TestWrapHidesCause(t *testing.T) {
	err := Wrap(connect.CodeUnavailable, "try later", errDB)

	if err.Message() != "try later" {
		t.Errorf("Message() = %q, want only the public message", err.Message())
	}

	if !errors.Is(err, errDB) {
		t.Error("cause is not reachable with errors.Is")
	}
}

func TestNotFoundDetail(t *testing.T) {
	err := NotFound("order", "42")

	if err.Code() != connect.CodeNotFound || err.Message() != `order "42" not found` {
		t.Fatalf("got %v %q", err.Code(), err.Message())
	}

	info := detail[*errdetails.ResourceInfo](t, err)
	if info.GetResourceType() != "order" || info.GetResourceName() != "42" {
		t.Errorf("ResourceInfo = %v", info)
	}
}

func TestInvalidFields(t *testing.T) {
	tests := []struct {
		name    string
		in      []FieldViolation
		wantMsg string
	}{
		{name: "none", wantMsg: "invalid argument"},
		{name: "one", in: []FieldViolation{{"sku", "is empty"}}, wantMsg: "invalid argument: sku: is empty"},
		{
			name:    "many",
			in:      []FieldViolation{{"sku", "is empty"}, {"qty", "must be positive"}, {"id", "bad"}},
			wantMsg: "invalid argument: sku: is empty (and 2 more)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := InvalidFields(tt.in...)

			if err.Code() != connect.CodeInvalidArgument || err.Message() != tt.wantMsg {
				t.Errorf("got %v %q, want %q", err.Code(), err.Message(), tt.wantMsg)
			}

			br := detail[*errdetails.BadRequest](t, err)
			if len(br.GetFieldViolations()) != len(tt.in) {
				t.Errorf("BadRequest has %d violations, want %d", len(br.GetFieldViolations()), len(tt.in))
			}
		})
	}
}

func TestSanitize(t *testing.T) {
	public := NotFound("order", "1")
	clientCode := connect.NewError(connect.CodeFailedPrecondition, errors.New("order is closed"))
	wire := connect.NewWireError(connect.CodeNotFound, errors.New("upstream detail"))

	tests := []struct {
		name         string
		in           error
		wantCode     connect.Code
		wantMsg      string
		wantReplaced bool
	}{
		{name: "public", in: public, wantCode: connect.CodeNotFound, wantMsg: public.Message()},
		{name: "wrapped public", in: fmt.Errorf("load: %w", public), wantCode: connect.CodeNotFound, wantMsg: public.Message()},
		{name: "client code", in: clientCode, wantCode: connect.CodeFailedPrecondition, wantMsg: "order is closed"},
		{name: "deadline", in: fmt.Errorf("query: %w", context.DeadlineExceeded), wantCode: connect.CodeDeadlineExceeded, wantMsg: "deadline exceeded"},
		{name: "canceled", in: context.Canceled, wantCode: connect.CodeCanceled, wantMsg: "canceled"},
		{name: "plain error", in: errDB, wantCode: connect.CodeInternal, wantMsg: internalMsg, wantReplaced: true},
		{
			name: "internal connect error", in: connect.NewError(connect.CodeInternal, errDB),
			wantCode: connect.CodeInternal, wantMsg: internalMsg, wantReplaced: true,
		},
		{name: "wire error", in: wire, wantCode: connect.CodeInternal, wantMsg: internalMsg, wantReplaced: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, replaced := Sanitize(tt.in)

			if got.Code() != tt.wantCode || got.Message() != tt.wantMsg || replaced != tt.wantReplaced {
				t.Errorf("Sanitize() = %v %q replaced=%v, want %v %q replaced=%v",
					got.Code(), got.Message(), replaced, tt.wantCode, tt.wantMsg, tt.wantReplaced)
			}

			if !errors.Is(got, tt.in) && !errors.Is(tt.in, got) {
				t.Error("result lost the original error for logs")
			}
		})
	}

	if got, _ := Sanitize(nil); got != nil {
		t.Errorf("Sanitize(nil) = %v", got)
	}
}

func detail[T any](t *testing.T, err *connect.Error) T {
	t.Helper()

	for _, d := range err.Details() {
		v, derr := d.Value()
		if derr != nil {
			t.Fatal(derr)
		}

		if typed, ok := v.(T); ok {
			return typed
		}
	}

	var zero T
	t.Fatalf("no %T detail in %v", zero, err)

	return zero
}
