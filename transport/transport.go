// Package transport serves Connect handlers over HTTP/2 and HTTP/1.1, with
// REST/JSON transcoding from google.api.http annotations.
//
// One handler serves the gRPC, gRPC-Web, Connect, and REST protocols. The
// server uses TLS, or h2c (HTTP/2 without TLS) when no TLS config is given.
package transport

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"connectrpc.com/vanguard"
)

// Route is one Connect service handler, as returned by a generated
// New<Service>Handler function.
type Route struct {
	Path    string
	Handler http.Handler
}

// NewHandler returns one handler for all routes. Each route path must be the
// path of a service whose proto descriptor is registered, which the generated
// code does. maxMessageBytes limits each message that Vanguard buffers to
// transcode. A request that Vanguard passes through unbuffered is limited only
// by the route handler, so build handlers with connect.WithReadMaxBytes.
func NewHandler(routes []Route, maxMessageBytes uint32) (http.Handler, error) {
	services := make([]*vanguard.Service, 0, len(routes))
	seen := make(map[string]bool, len(routes))

	for _, r := range routes {
		switch {
		case r.Path == "" || r.Handler == nil:
			return nil, fmt.Errorf("transport: route %q has no path or no handler", r.Path)
		case seen[r.Path]:
			return nil, fmt.Errorf("transport: route %q is registered more than once", r.Path)
		}

		seen[r.Path] = true

		services = append(services, vanguard.NewService(r.Path, r.Handler))
	}

	h, err := vanguard.NewTranscoder(services,
		vanguard.WithDefaultServiceOptions(vanguard.WithMaxMessageBufferBytes(maxMessageBytes)))
	if err != nil {
		return nil, fmt.Errorf("transport: %w", err)
	}

	return h, nil
}

// Config configures a Server.
type Config struct {
	// Addr is the TCP listen address, for example ":8443".
	Addr string
	// TLS enables TLS. A nil TLS serves h2c and HTTP/1.1 without TLS.
	TLS               *tls.Config
	ReadHeaderTimeout time.Duration
	IdleTimeout       time.Duration
}

// Server is an HTTP server with a bound listener. Listen, Serve, and Shutdown
// must be called from one goroutine.
type Server struct {
	srv    *http.Server
	ln     net.Listener
	tls    bool
	served chan struct{}
	logger *slog.Logger
}

// Listen binds the listen address and returns a Server that does not accept
// connections until Serve.
func Listen(ctx context.Context, cfg Config, h http.Handler, logger *slog.Logger) (*Server, error) {
	var lc net.ListenConfig

	ln, err := lc.Listen(ctx, "tcp", cfg.Addr)
	if err != nil {
		return nil, fmt.Errorf("transport: listen: %w", err)
	}

	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)

	if cfg.TLS != nil {
		protocols.SetHTTP2(true)
	} else {
		protocols.SetUnencryptedHTTP2(true)
	}

	return &Server{
		srv: &http.Server{
			Handler:           h,
			TLSConfig:         cfg.TLS,
			Protocols:         protocols,
			ReadHeaderTimeout: cfg.ReadHeaderTimeout,
			IdleTimeout:       cfg.IdleTimeout,
			ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
		},
		ln:     ln,
		tls:    cfg.TLS != nil,
		logger: logger,
	}, nil
}

// Addr returns the bound address.
func (s *Server) Addr() net.Addr { return s.ln.Addr() }

// Serve accepts connections in a new goroutine. onError receives an error
// that stops serving before Shutdown; it is not called after Shutdown.
func (s *Server) Serve(onError func(error)) {
	s.served = make(chan struct{})

	go func() {
		defer close(s.served)

		var err error
		if s.tls {
			err = s.srv.ServeTLS(s.ln, "", "")
		} else {
			err = s.srv.Serve(s.ln)
		}

		if !errors.Is(err, http.ErrServerClosed) {
			onError(fmt.Errorf("transport: serve: %w", err))
		}
	}()

	s.logger.Info("rpc server serving", "addr", s.Addr().String(), "tls", s.tls)
}

// Shutdown stops accepting connections and waits for active calls until ctx
// ends. Then it closes the connections that remain, which cancels their calls.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.served == nil {
		if err := s.ln.Close(); err != nil {
			return fmt.Errorf("transport: close listener: %w", err)
		}

		return nil
	}

	err := s.srv.Shutdown(ctx)
	if err != nil {
		s.logger.WarnContext(ctx, "rpc server shutdown timed out; closing active calls", "error", err)
		err = errors.Join(fmt.Errorf("transport: shutdown: %w", err), s.srv.Close())
	}

	<-s.served

	return err
}
