package interceptor

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"go.uber.org/goleak"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	testv1 "github.com/sinni19/go-svclib/internal/gen/svclib/test/v1"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

const procedure = "/svclib.test.v1.EchoService/Echo"

var errPlain = errors.New("db: password=hunter2")

// request returns a handler-side request for msg. Set client to build a
// client-side request.
func request[T any](msg *T, client bool) connect.AnyRequest {
	req := connect.NewRequest(msg)
	if client {
		return req
	}

	return serverRequest{AnyRequest: req}
}

// serverRequest reports a handler-side Spec, which connect.NewRequest cannot set.
type serverRequest struct{ connect.AnyRequest }

func (serverRequest) Spec() connect.Spec { return connect.Spec{Procedure: procedure} }
func (serverRequest) Peer() connect.Peer { return connect.Peer{Protocol: connect.ProtocolGRPC} }

// fakeConn is a handler stream that receives msgs in order, then io.EOF.
type fakeConn struct {
	msgs []any
	sent []any
}

func (c *fakeConn) Spec() connect.Spec {
	return connect.Spec{Procedure: procedure, StreamType: connect.StreamTypeBidi}
}
func (c *fakeConn) Peer() connect.Peer           { return connect.Peer{Protocol: connect.ProtocolConnect} }
func (c *fakeConn) RequestHeader() http.Header   { return http.Header{} }
func (c *fakeConn) ResponseHeader() http.Header  { return http.Header{} }
func (c *fakeConn) ResponseTrailer() http.Header { return http.Header{} }
func (c *fakeConn) Send(msg any) error           { c.sent = append(c.sent, msg); return nil }

func (c *fakeConn) Receive(msg any) error {
	if len(c.msgs) == 0 {
		return io.EOF
	}

	next := c.msgs[0]
	c.msgs = c.msgs[1:]

	dst, ok := msg.(*testv1.EchoStreamRequest)
	src, ok2 := next.(*testv1.EchoStreamRequest)
	if ok && ok2 {
		dst.Message = src.GetMessage()
	}

	return nil
}

// codeOf returns 0 for a nil error; connect.CodeOf returns CodeUnknown.
func codeOf(err error) connect.Code {
	if err == nil {
		return 0
	}

	return connect.CodeOf(err)
}

func newLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer

	return slog.New(slog.NewJSONHandler(&buf, nil)), &buf
}

func unary(err error, panicWith any) connect.UnaryFunc {
	return func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
		if panicWith != nil {
			panic(panicWith)
		}

		if err != nil {
			return nil, err
		}

		return connect.NewResponse(&emptypb.Empty{}), nil
	}
}

func stream(err error, panicWith any) connect.StreamingHandlerFunc {
	return func(context.Context, connect.StreamingHandlerConn) error {
		if panicWith != nil {
			panic(panicWith)
		}

		return err
	}
}

func TestClientCallsPassThrough(t *testing.T) {
	logger, logs := newLogger()
	i := Recover(logger)

	called := false
	next := func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
		called = true

		return nil, errPlain
	}

	_, err := i.WrapUnary(next)(t.Context(), request(&emptypb.Empty{}, true))

	if !called || !errors.Is(err, errPlain) {
		t.Errorf("client call changed: called=%v err=%v", called, err)
	}

	client := func(context.Context, connect.Spec) connect.StreamingClientConn { return nil }
	if i.WrapStreamingClient(client)(t.Context(), connect.Spec{}) != nil {
		t.Error("WrapStreamingClient changed the client func")
	}

	if logs.Len() != 0 {
		t.Errorf("client call was logged: %s", logs)
	}
}

func TestRecover(t *testing.T) {
	logger, logs := newLogger()
	i := Recover(logger)

	t.Run("unary panic", func(t *testing.T) {
		_, err := i.WrapUnary(unary(nil, "boom"))(t.Context(), request(&emptypb.Empty{}, false))
		if connect.CodeOf(err) != connect.CodeInternal {
			t.Errorf("err = %v, want Internal", err)
		}
	})

	t.Run("stream panic", func(t *testing.T) {
		err := i.WrapStreamingHandler(stream(nil, "boom"))(t.Context(), &fakeConn{})
		if connect.CodeOf(err) != connect.CodeInternal {
			t.Errorf("err = %v, want Internal", err)
		}
	})

	t.Run("no panic", func(t *testing.T) {
		_, err := i.WrapUnary(unary(errPlain, nil))(t.Context(), request(&emptypb.Empty{}, false))
		if !errors.Is(err, errPlain) {
			t.Errorf("err = %v, want the handler error unchanged", err)
		}

		if err := i.WrapStreamingHandler(stream(nil, nil))(t.Context(), &fakeConn{}); err != nil {
			t.Errorf("stream err = %v", err)
		}
	})

	if n := strings.Count(logs.String(), `"msg":"rpc handler panicked"`); n != 2 {
		t.Errorf("panic logged %d times, want 2", n)
	}

	if !strings.Contains(logs.String(), `"stack":`) {
		t.Error("panic stack is not logged")
	}
}

func TestRequestLog(t *testing.T) {
	tests := []struct {
		name     string
		run      func(i connect.Interceptor, ctx context.Context)
		wantCode string
		wantProt string
	}{
		{
			name: "unary ok",
			run: func(i connect.Interceptor, ctx context.Context) {
				_, _ = i.WrapUnary(unary(nil, nil))(ctx, request(&emptypb.Empty{}, false))
			},
			wantCode: `"code":"ok"`, wantProt: `"protocol":"grpc"`,
		},
		{
			name: "unary error",
			run: func(i connect.Interceptor, ctx context.Context) {
				_, _ = i.WrapUnary(unary(connect.NewError(connect.CodeNotFound, nil), nil))(ctx, request(&emptypb.Empty{}, false))
			},
			wantCode: `"code":"not_found"`, wantProt: `"protocol":"grpc"`,
		},
		{
			name: "stream error",
			run: func(i connect.Interceptor, ctx context.Context) {
				_ = i.WrapStreamingHandler(stream(connect.NewError(connect.CodeAborted, nil), nil))(ctx, &fakeConn{})
			},
			wantCode: `"code":"aborted"`, wantProt: `"protocol":"connect"`,
		},
		{
			name: "unary panic is logged and continues",
			run: func(i connect.Interceptor, ctx context.Context) {
				defer func() {
					if recover() == nil {
						t.Error("RequestLog stopped the panic; Recover must receive it")
					}
				}()

				_, _ = i.WrapUnary(unary(nil, "boom"))(ctx, request(&emptypb.Empty{}, false))
			},
			wantCode: `"code":"internal"`, wantProt: `"protocol":"grpc"`,
		},
		{
			name: "stream panic is logged and continues",
			run: func(i connect.Interceptor, ctx context.Context) {
				defer func() { _ = recover() }()

				_ = i.WrapStreamingHandler(stream(nil, "boom"))(ctx, &fakeConn{})
			},
			wantCode: `"code":"internal"`, wantProt: `"protocol":"connect"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, logs := newLogger()
			tt.run(RequestLog(logger), t.Context())

			out := logs.String()
			for _, want := range []string{`"msg":"rpc finished"`, `"procedure":"` + procedure + `"`, tt.wantCode, tt.wantProt, `"duration":`} {
				if !strings.Contains(out, want) {
					t.Errorf("log %s lacks %s", out, want)
				}
			}
		})
	}
}

func TestSanitize(t *testing.T) {
	public := connect.NewError(connect.CodeNotFound, errors.New("order not found"))

	tests := []struct {
		name     string
		err      error
		wantCode connect.Code
		wantLog  bool
	}{
		{name: "nil", err: nil},
		{name: "client code kept", err: public, wantCode: connect.CodeNotFound},
		{name: "plain error hidden", err: errPlain, wantCode: connect.CodeInternal, wantLog: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, logs := newLogger()
			i := Sanitize(logger)

			_, uerr := i.WrapUnary(unary(tt.err, nil))(t.Context(), request(&emptypb.Empty{}, false))
			serr := i.WrapStreamingHandler(stream(tt.err, nil))(t.Context(), &fakeConn{})

			for _, err := range []error{uerr, serr} {
				if tt.err == nil {
					if err != nil {
						t.Errorf("err = %v, want nil", err)
					}

					continue
				}

				if connect.CodeOf(err) != tt.wantCode {
					t.Errorf("code = %v, want %v", connect.CodeOf(err), tt.wantCode)
				}

				// The secret must never reach the client message.
				if strings.Contains(err.Error(), "hunter2") {
					t.Errorf("client error %q holds the secret", err)
				}
			}

			if got := strings.Contains(logs.String(), "hunter2"); got != tt.wantLog {
				t.Errorf("original error logged = %v, want %v", got, tt.wantLog)
			}
		})
	}
}

func TestDeadline(t *testing.T) {
	const def, maxT = time.Second, 5 * time.Second

	tests := []struct {
		name   string
		client time.Duration // 0 means no client deadline
		want   time.Duration
	}{
		{name: "default when client sets none", want: def},
		{name: "client deadline kept", client: 3 * time.Second, want: 3 * time.Second},
		{name: "client deadline capped", client: time.Hour, want: maxT},
	}

	i := Deadline(def, maxT)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			if tt.client > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.client)
				defer cancel()
			}

			var got time.Duration

			next := func(ctx context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
				d, ok := ctx.Deadline()
				if !ok {
					t.Fatal("handler context has no deadline")
				}

				got = time.Until(d)

				return nil, nil
			}

			_, _ = i.WrapUnary(next)(ctx, request(&emptypb.Empty{}, false))

			if diff := tt.want - got; diff < 0 || diff > 100*time.Millisecond {
				t.Errorf("deadline in %v, want about %v", got, tt.want)
			}
		})
	}

	t.Run("streams have no server deadline", func(t *testing.T) {
		next := func(ctx context.Context, _ connect.StreamingHandlerConn) error {
			if _, ok := ctx.Deadline(); ok {
				t.Error("stream context has a deadline")
			}

			return nil
		}

		_ = i.WrapStreamingHandler(next)(t.Context(), &fakeConn{})
	})
}

func TestValidateUnary(t *testing.T) {
	v, err := protovalidate.New()
	if err != nil {
		t.Fatal(err)
	}

	i := Validate(v)

	tests := []struct {
		name     string
		req      connect.AnyRequest
		wantCode connect.Code
	}{
		{name: "valid", req: request(&testv1.EchoRequest{Message: "hi"}, false)},
		{name: "rule violation", req: request(&testv1.EchoRequest{}, false), wantCode: connect.CodeInvalidArgument},
		{name: "message without rules", req: request(&emptypb.Empty{}, false)},
		{name: "not a proto message", req: request(new(string), false)},
		{name: "broken rule", req: request(&testv1.EchoRequest{Message: "hi"}, false), wantCode: connect.CodeInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ic := i
			if tt.name == "broken rule" {
				ic = Validate(failingValidator{})
			}

			called := false
			next := func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
				called = true

				return nil, nil
			}

			_, err := ic.WrapUnary(next)(t.Context(), tt.req)

			if codeOf(err) != tt.wantCode {
				t.Errorf("err = %v, want code %v", err, tt.wantCode)
			}

			// An invalid request must never reach the handler.
			if called == (tt.wantCode != 0) {
				t.Errorf("handler called = %v with code %v", called, tt.wantCode)
			}
		})
	}
}

// failingValidator fails with an error that is not a ValidationError.
type failingValidator struct{}

func (failingValidator) Validate(proto.Message, ...protovalidate.ValidationOption) error {
	return errors.New("compile: bad rule")
}

func TestValidateStream(t *testing.T) {
	v, err := protovalidate.New()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		msgs     []any
		wantRecv int
		wantCode connect.Code
	}{
		{name: "all valid", msgs: []any{&testv1.EchoStreamRequest{Message: "a"}, &testv1.EchoStreamRequest{Message: "b"}}, wantRecv: 2},
		{name: "invalid second message", msgs: []any{&testv1.EchoStreamRequest{Message: "a"}, &testv1.EchoStreamRequest{}}, wantRecv: 1, wantCode: connect.CodeInvalidArgument},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := 0
			handler := func(_ context.Context, conn connect.StreamingHandlerConn) error {
				for {
					msg := new(testv1.EchoStreamRequest)

					err := conn.Receive(msg)
					if errors.Is(err, io.EOF) {
						return nil
					}

					if err != nil {
						return err
					}

					got++
				}
			}

			err := Validate(v).WrapStreamingHandler(handler)(t.Context(), &fakeConn{msgs: tt.msgs})

			if got != tt.wantRecv || codeOf(err) != tt.wantCode {
				t.Errorf("received %d, err %v; want %d, code %v", got, err, tt.wantRecv, tt.wantCode)
			}
		})
	}
}
