package svc

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/genproto/googleapis/rpc/errdetails"

	testv1 "github.com/sinni19/go-svclib/internal/gen/svclib/test/v1"
	"github.com/sinni19/go-svclib/internal/gen/svclib/test/v1/testv1connect"
)

// echoService implements the test EchoService. A message selects a behavior:
// "panic", "fail" (a plain error with a secret), "slow" (waits for its
// deadline, at most 3s), or "deadline" (returns the remaining time).
type echoService struct {
	BaseService

	ignoreOpts bool
	// started is closed when a "block" stream message arrives.
	started chan struct{}
}

func (*echoService) Name() string { return "echo" }

func (e *echoService) Handlers(opts ...connect.HandlerOption) []Handler {
	if e.ignoreOpts {
		opts = nil
	}

	return []Handler{NewHandler(testv1connect.NewEchoServiceHandler(e, opts...))}
}

var errSecret = errors.New("secret: password=hunter2")

func (*echoService) Echo(ctx context.Context, req *connect.Request[testv1.EchoRequest]) (*connect.Response[testv1.EchoResponse], error) {
	msg := req.Msg.GetMessage()

	switch msg {
	case "panic":
		panic("boom")
	case "fail":
		return nil, errSecret
	case "slow":
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(3 * time.Second):
			return nil, errors.New("no deadline")
		}
	case "deadline":
		d, _ := ctx.Deadline()
		msg = time.Until(d).Round(time.Second).String()
	}

	return connect.NewResponse(&testv1.EchoResponse{Message: msg}), nil
}

func (e *echoService) EchoStream(ctx context.Context, stream *connect.BidiStream[testv1.EchoStreamRequest, testv1.EchoStreamResponse]) error {
	for {
		req, err := stream.Receive()
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			return err
		}

		switch req.GetMessage() {
		case "panic":
			panic("stream boom")
		case "block":
			close(e.started)
			<-ctx.Done()

			return ctx.Err()
		}

		if err := stream.Send(&testv1.EchoStreamResponse{Message: req.GetMessage()}); err != nil {
			return err
		}
	}
}

// syncBuffer is a log writer that tests read while the App writes.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

// startEcho launches an App with the echo service on a free port and returns
// its base URL. The App stops at the end of the test.
func startEcho(t *testing.T, logs io.Writer, extra ...Option) (*App, string) {
	t.Helper()

	a := newApp(t, append([]Option{Addr("127.0.0.1:0"), LogWriter(logs), Services(&echoService{started: make(chan struct{})})}, extra...)...)
	res := launch(t, a, StatusRunning)

	t.Cleanup(func() {
		if err := a.Shutdown(context.Background()); err != nil {
			t.Errorf("Shutdown() error = %v", err)
		}

		if err := <-res; err != nil {
			t.Errorf("Launch() error = %v", err)
		}
	})

	return a, "http://" + a.RPCAddr().String()
}

// h2cClient speaks HTTP/2 without TLS, as gRPC requires.
func h2cClient() *http.Client {
	p := new(http.Protocols)
	p.SetUnencryptedHTTP2(true)

	return &http.Client{Transport: &http.Transport{Protocols: p}}
}

func TestOneRPCOverEveryProtocol(t *testing.T) {
	_, url := startEcho(t, io.Discard)

	clients := map[string]testv1connect.EchoServiceClient{
		"connect":  testv1connect.NewEchoServiceClient(http.DefaultClient, url),
		"grpc":     testv1connect.NewEchoServiceClient(h2cClient(), url, connect.WithGRPC()),
		"grpc-web": testv1connect.NewEchoServiceClient(http.DefaultClient, url, connect.WithGRPCWeb()),
	}

	for name, c := range clients {
		t.Run(name, func(t *testing.T) {
			res, err := c.Echo(t.Context(), connect.NewRequest(&testv1.EchoRequest{Message: "hi"}))
			if err != nil {
				t.Fatal(err)
			}

			if res.Msg.GetMessage() != "hi" {
				t.Errorf("message = %q", res.Msg.GetMessage())
			}
		})
	}

	t.Run("rest", func(t *testing.T) {
		code, body := postJSON(t, url+"/v1/echo", `{"message":"hi"}`)
		if code != http.StatusOK || !strings.Contains(body, `"message":"hi"`) {
			t.Errorf("POST /v1/echo = %d %s", code, body)
		}
	})
}

func postJSON(t *testing.T, url, body string) (int, string) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()

	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}

	return res.StatusCode, string(b)
}

func TestUnaryInterceptors(t *testing.T) {
	logs := new(syncBuffer)
	_, url := startEcho(t, logs, RPCDefaultTimeout(50*time.Millisecond), RPCMaxTimeout(2*time.Second))
	c := testv1connect.NewEchoServiceClient(h2cClient(), url, connect.WithGRPC())

	tests := []struct {
		name     string
		msg      string
		timeout  time.Duration
		wantCode connect.Code
		wantMsg  string
		// wantFast requires the call to end within 500ms.
		wantFast bool
	}{
		{name: "panic is internal", msg: "panic", wantCode: connect.CodeInternal, wantMsg: "internal error"},
		{name: "plain error is sanitized", msg: "fail", wantCode: connect.CodeInternal, wantMsg: "internal error"},
		{name: "invalid request", msg: "", wantCode: connect.CodeInvalidArgument, wantMsg: "message"},
		{name: "default deadline applies", msg: "slow", wantCode: connect.CodeDeadlineExceeded, wantFast: true},
		{name: "client deadline is capped", msg: "deadline", timeout: time.Hour, wantMsg: "2s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			if tt.timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.timeout)
				defer cancel()
			}

			start := time.Now()
			res, err := c.Echo(ctx, connect.NewRequest(&testv1.EchoRequest{Message: tt.msg}))

			// The client sets no deadline, so only the 50ms server default can end the call early.
			if tt.wantFast && time.Since(start) > 500*time.Millisecond {
				t.Fatalf("Echo() took %v; the default deadline did not apply", time.Since(start))
			}

			if tt.wantCode == 0 {
				if err != nil || res.Msg.GetMessage() != tt.wantMsg {
					t.Fatalf("Echo() = %v, %v; want %q", res, err, tt.wantMsg)
				}

				return
			}

			var ce *connect.Error
			if !errors.As(err, &ce) || ce.Code() != tt.wantCode || !strings.Contains(ce.Message(), tt.wantMsg) {
				t.Fatalf("Echo() error = %v, want %v containing %q", err, tt.wantCode, tt.wantMsg)
			}

			if strings.Contains(ce.Message(), "hunter2") {
				t.Error("a server secret reached the client")
			}
		})
	}

	out := logs.String()
	for _, want := range []string{`"msg":"rpc handler panicked"`, `"stack":`, "hunter2", `"msg":"rpc finished"`, `"code":"internal"`} {
		if !strings.Contains(out, want) {
			t.Errorf("logs lack %s", want)
		}
	}
}

func TestInvalidRequestHasFieldDetail(t *testing.T) {
	_, url := startEcho(t, io.Discard)
	c := testv1connect.NewEchoServiceClient(http.DefaultClient, url)

	_, err := c.Echo(t.Context(), connect.NewRequest(&testv1.EchoRequest{Message: strings.Repeat("x", 101)}))

	var ce *connect.Error
	if !errors.As(err, &ce) {
		t.Fatalf("Echo() error = %v", err)
	}

	for _, d := range ce.Details() {
		if br, ok := must(d.Value()).(*errdetails.BadRequest); ok {
			if got := br.GetFieldViolations()[0].GetField(); got != "message" {
				t.Errorf("field = %q, want message", got)
			}

			return
		}
	}

	t.Errorf("no BadRequest detail in %v", ce)
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}

	return v
}

func TestStreamInterceptors(t *testing.T) {
	logs := new(syncBuffer)
	_, url := startEcho(t, logs)
	c := testv1connect.NewEchoServiceClient(h2cClient(), url, connect.WithGRPC())

	t.Run("echo", func(t *testing.T) {
		s := c.EchoStream(t.Context())
		for _, m := range []string{"a", "b"} {
			if err := s.Send(&testv1.EchoStreamRequest{Message: m}); err != nil {
				t.Fatal(err)
			}

			res, err := s.Receive()
			if err != nil || res.GetMessage() != m {
				t.Fatalf("Receive() = %v, %v; want %q", res, err, m)
			}
		}

		if err := s.CloseRequest(); err != nil {
			t.Fatal(err)
		}

		if _, err := s.Receive(); !errors.Is(err, io.EOF) {
			t.Errorf("Receive() after close = %v, want EOF", err)
		}
	})

	errCases := []struct {
		name     string
		msgs     []string
		wantCode connect.Code
	}{
		{name: "invalid message mid-stream", msgs: []string{"ok", ""}, wantCode: connect.CodeInvalidArgument},
		{name: "panic", msgs: []string{"panic"}, wantCode: connect.CodeInternal},
	}

	for _, tt := range errCases {
		t.Run(tt.name, func(t *testing.T) {
			s := c.EchoStream(t.Context())
			for _, m := range tt.msgs {
				_ = s.Send(&testv1.EchoStreamRequest{Message: m})
			}

			_ = s.CloseRequest()

			var err error
			for err == nil {
				_, err = s.Receive()
			}

			if connect.CodeOf(err) != tt.wantCode {
				t.Errorf("stream error = %v, want %v", err, tt.wantCode)
			}

			_ = s.CloseResponse()
		})
	}

	if !strings.Contains(logs.String(), `"procedure":"/svclib.test.v1.EchoService/EchoStream"`) {
		t.Error("stream calls are not logged")
	}
}

func TestShutdownEndsOpenStreams(t *testing.T) {
	svc := &echoService{started: make(chan struct{})}
	a := newApp(t, Addr("127.0.0.1:0"), ShutdownTimeout(200*time.Millisecond), Services(svc))
	res := launch(t, a, StatusRunning)

	c := testv1connect.NewEchoServiceClient(h2cClient(), "http://"+a.RPCAddr().String(), connect.WithGRPC())
	s := c.EchoStream(t.Context())

	if err := s.Send(&testv1.EchoStreamRequest{Message: "block"}); err != nil {
		t.Fatal(err)
	}

	<-svc.started

	start := time.Now()

	err := a.Shutdown(context.Background())
	if err == nil || !strings.Contains(err.Error(), "shutdown") {
		t.Errorf("Shutdown() error = %v, want the server shutdown timeout", err)
	}

	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Shutdown took %v with an open stream", elapsed)
	}

	if err := <-res; err == nil {
		t.Error("Launch() error = nil, want the server shutdown timeout")
	}

	_ = s.CloseRequest()
	_ = s.CloseResponse()
}

func TestHandlersMustUseAppOptions(t *testing.T) {
	a := newApp(t, Addr("127.0.0.1:0"), Services(&echoService{ignoreOpts: true}))

	res := make(chan error, 1)

	go func() { res <- a.Launch() }()

	select {
	case err := <-res:
		if !errors.Is(err, errChainIgnored) || !strings.Contains(err.Error(), `"echo"`) {
			t.Errorf("Launch() error = %v, want errChainIgnored for echo", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Launch did not fail; a service without the interceptor chain was served")

		// Shutdown returns after Launch, so no goroutine leaks.
		_ = a.Shutdown(context.Background())
	}
}

func TestPortInUseFailsBeforeOnStart(t *testing.T) {
	var lc net.ListenConfig

	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	rec := &recorder{}
	a := newApp(t, Addr(ln.Addr().String()), Services(&echoService{}, &fakeService{name: "b", rec: rec}))

	if err := a.Launch(); err == nil || !strings.Contains(err.Error(), "listen") {
		t.Fatalf("Launch() error = %v, want a listen error", err)
	}

	got := strings.Join(rec.list(), " ")
	if strings.Contains(got, "OnStart") || !strings.Contains(got, "b.OnStop") {
		t.Errorf("events = %q, want OnStop without OnStart", got)
	}
}

func TestNoHandlersNoServer(t *testing.T) {
	a := newApp(t, Services(&fakeService{name: "a", rec: &recorder{}}))
	res := launch(t, a, StatusRunning)

	if a.RPCAddr() != nil {
		t.Errorf("RPCAddr() = %v, want nil without handlers", a.RPCAddr())
	}

	if err := a.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	if err := <-res; err != nil {
		t.Fatal(err)
	}
}

func TestTLSServesHTTP2(t *testing.T) {
	cfg := testTLS(t)
	a := newApp(t, withoutInsecure([]Option{Addr("127.0.0.1:0"), TLS(cfg), Services(&echoService{})})...)
	res := launch(t, a, StatusRunning)

	pool := x509.NewCertPool()
	pool.AddCert(cfg.Certificates[0].Leaf)

	p := new(http.Protocols)
	p.SetHTTP2(true)
	hc := &http.Client{Transport: &http.Transport{Protocols: p, TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}}

	c := testv1connect.NewEchoServiceClient(hc, "https://"+a.RPCAddr().String(), connect.WithGRPC())
	if _, err := c.Echo(t.Context(), connect.NewRequest(&testv1.EchoRequest{Message: "tls"})); err != nil {
		t.Errorf("Echo() over TLS error = %v", err)
	}

	hc.CloseIdleConnections()

	if err := a.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	if err := <-res; err != nil {
		t.Fatal(err)
	}
}

// testTLS returns a TLS config with a self-signed certificate for 127.0.0.1.
func testTLS(t *testing.T) *tls.Config {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},

		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}},
		MinVersion:   tls.VersionTLS12,
	}
}
