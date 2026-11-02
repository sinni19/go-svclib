package transport

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
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"go.uber.org/goleak"

	testv1 "github.com/sinni19/go-svclib/internal/gen/svclib/test/v1"
	"github.com/sinni19/go-svclib/internal/gen/svclib/test/v1/testv1connect"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// echo returns the request message. "block" waits until release is closed.
type echo struct {
	testv1connect.UnimplementedEchoServiceHandler

	started chan struct{}
	release chan struct{}
}

func (e *echo) Echo(ctx context.Context, req *connect.Request[testv1.EchoRequest]) (*connect.Response[testv1.EchoResponse], error) {
	if req.Msg.GetMessage() == "block" {
		close(e.started)

		select {
		case <-e.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return connect.NewResponse(&testv1.EchoResponse{Message: req.Msg.GetMessage()}), nil
}

func echoRoute(e *echo, opts ...connect.HandlerOption) Route {
	path, h := testv1connect.NewEchoServiceHandler(e, opts...)

	return Route{Path: path, Handler: h}
}

func TestNewHandler(t *testing.T) {
	good := echoRoute(&echo{})

	tests := []struct {
		name    string
		routes  []Route
		wantErr string
	}{
		{name: "valid", routes: []Route{good}},
		{name: "no routes", routes: nil},
		{name: "empty path", routes: []Route{{Handler: good.Handler}}, wantErr: "no path or no handler"},
		{name: "nil handler", routes: []Route{{Path: good.Path}}, wantErr: "no path or no handler"},
		{name: "duplicate", routes: []Route{good, good}, wantErr: "more than once"},
		{name: "unknown service", routes: []Route{{Path: "/no.such.v1.Service/", Handler: good.Handler}}, wantErr: "transport:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := NewHandler(tt.routes, 1<<20)

			if tt.wantErr == "" {
				if err != nil || h == nil {
					t.Fatalf("NewHandler() = %v, %v", h, err)
				}

				return
			}

			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("NewHandler() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

// The App pairs maxMessageBytes with connect.WithReadMaxBytes; REST JSON
// passes through Vanguard unbuffered, so the handler option enforces the limit.
func TestMaxMessageBytesLimitsREST(t *testing.T) {
	h, err := NewHandler([]Route{echoRoute(&echo{}, connect.WithReadMaxBytes(64))}, 64)
	if err != nil {
		t.Fatal(err)
	}

	srv := start(t, Config{Addr: "127.0.0.1:0"}, h)

	small := post(t, srv, `{"message":"hi"}`)
	big := post(t, srv, `{"message":"`+strings.Repeat("x", 200)+`"}`)

	if small != http.StatusOK || big == http.StatusOK {
		t.Errorf("status small=%d big=%d; want 200 and an error", small, big)
	}
}

func post(t *testing.T, srv *Server, body string) int {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+srv.Addr().String()+"/v1/echo", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()

	return res.StatusCode
}

// start listens and serves h; the server shuts down at the end of the test.
func start(t *testing.T, cfg Config, h http.Handler) *Server {
	t.Helper()

	srv, err := Listen(t.Context(), cfg, h, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}

	srv.Serve(func(err error) { t.Errorf("serve error: %v", err) })

	t.Cleanup(func() {
		http.DefaultClient.CloseIdleConnections()

		if err := srv.Shutdown(context.Background()); err != nil {
			t.Errorf("Shutdown() error = %v", err)
		}
	})

	return srv
}

func TestListenFails(t *testing.T) {
	_, err := Listen(t.Context(), Config{Addr: "256.0.0.1:1"}, http.NotFoundHandler(), slog.New(slog.DiscardHandler))
	if err == nil || !strings.Contains(err.Error(), "listen") {
		t.Errorf("Listen() error = %v, want a listen error", err)
	}
}

func TestShutdownWithoutServe(t *testing.T) {
	srv, err := Listen(t.Context(), Config{Addr: "127.0.0.1:0"}, http.NotFoundHandler(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}

	addr := srv.Addr().String()

	if err := srv.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	// The port is free again, so a failed startup does not keep it.
	var lc net.ListenConfig

	ln, err := lc.Listen(t.Context(), "tcp", addr)
	if err != nil {
		t.Fatalf("port still bound: %v", err)
	}

	_ = ln.Close()

	if err := srv.Shutdown(t.Context()); err == nil {
		t.Error("second Shutdown() closed the listener twice without an error")
	}
}

func TestServeReportsError(t *testing.T) {
	srv, err := Listen(t.Context(), Config{Addr: "127.0.0.1:0"}, http.NotFoundHandler(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}

	// A closed listener makes Serve fail before Shutdown.
	_ = srv.ln.Close()

	got := make(chan error, 1)
	srv.Serve(func(err error) { got <- err })

	select {
	case err := <-got:
		if !strings.Contains(err.Error(), "serve") {
			t.Errorf("error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not report the error")
	}

	_ = srv.Shutdown(t.Context())
}

func TestShutdownTimeoutClosesActiveCalls(t *testing.T) {
	e := &echo{started: make(chan struct{}), release: make(chan struct{})}
	defer close(e.release)

	h, err := NewHandler([]Route{echoRoute(e)}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}

	var logs syncBuffer

	srv, err := Listen(t.Context(), Config{Addr: "127.0.0.1:0"}, h, slog.New(slog.NewJSONHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}

	srv.Serve(func(err error) { t.Errorf("serve error: %v", err) })

	c := testv1connect.NewEchoServiceClient(http.DefaultClient, "http://"+srv.Addr().String())
	callErr := make(chan error, 1)

	go func() {
		_, err := c.Echo(context.Background(), connect.NewRequest(&testv1.EchoRequest{Message: "block"}))
		callErr <- err
	}()

	<-e.started

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	err = srv.Shutdown(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Shutdown() error = %v, want DeadlineExceeded", err)
	}

	if err := <-callErr; err == nil {
		t.Error("active call succeeded after a forced close")
	}

	if !strings.Contains(logs.String(), "closing active calls") {
		t.Error("forced close is not logged")
	}

	http.DefaultClient.CloseIdleConnections()
}

func TestTLS(t *testing.T) {
	cfg, pool := testTLS(t)

	h, err := NewHandler([]Route{echoRoute(&echo{})}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}

	srv := start(t, Config{Addr: "127.0.0.1:0", TLS: cfg, ReadHeaderTimeout: time.Second, IdleTimeout: time.Second}, h)

	tr := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}, ForceAttemptHTTP2: true}
	defer tr.CloseIdleConnections()

	c := testv1connect.NewEchoServiceClient(&http.Client{Transport: tr}, "https://"+srv.Addr().String(), connect.WithGRPC())

	res, err := c.Echo(t.Context(), connect.NewRequest(&testv1.EchoRequest{Message: "tls"}))
	if err != nil || res.Msg.GetMessage() != "tls" {
		t.Fatalf("Echo() = %v, %v", res, err)
	}

	// A plaintext client must fail against the TLS port.
	plain := testv1connect.NewEchoServiceClient(http.DefaultClient, "http://"+srv.Addr().String())
	if _, err := plain.Echo(t.Context(), connect.NewRequest(&testv1.EchoRequest{Message: "x"})); err == nil {
		t.Error("plaintext call to the TLS port succeeded")
	}
}

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

func testTLS(t *testing.T) (*tls.Config, *x509.CertPool) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test"},
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
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

	pool := x509.NewCertPool()
	pool.AddCert(leaf)

	return &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}},
		MinVersion:   tls.VersionTLS12,
	}, pool
}
