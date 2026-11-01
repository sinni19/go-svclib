package svc

import (
	"context"
	"log/slog"
)

// Service is a unit of business logic that the App hosts.
//
// Embed BaseService and override only the methods you need. A type that does
// not embed BaseService breaks when a later version adds a method.
type Service interface {
	// Name identifies the service in logs. It must be unique in an App.
	Name() string

	// OnInit prepares the service. A service whose OnInit returns nil always
	// gets OnStop, also when a later service fails to start.
	OnInit(ctx context.Context, d *Deps) error

	// OnStart runs after every service is initialized.
	OnStart(ctx context.Context) error

	// OnStop releases what OnInit and OnStart acquired. It must also work when
	// OnStart was never called. Services stop in reverse registration order.
	OnStop(ctx context.Context) error

	// Ready reports whether the service can accept work.
	Ready(ctx context.Context) error

	// Live reports whether the process is healthy. An error means the process
	// cannot recover and must restart.
	Live(ctx context.Context) error
}

// Deps holds the dependencies that the App gives to each service in OnInit.
type Deps struct {
	// Logger has the attribute service=<Name()>.
	Logger *slog.Logger
}

// BaseService implements every Service method except Name with a default that
// does nothing and reports healthy.
type BaseService struct{}

// OnInit does nothing.
func (BaseService) OnInit(context.Context, *Deps) error { return nil }

// OnStart does nothing.
func (BaseService) OnStart(context.Context) error { return nil }

// OnStop does nothing.
func (BaseService) OnStop(context.Context) error { return nil }

// Ready reports ready.
func (BaseService) Ready(context.Context) error { return nil }

// Live reports live.
func (BaseService) Live(context.Context) error { return nil }
