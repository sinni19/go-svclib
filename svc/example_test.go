package svc_test

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/sinni19/go-svclib/svc"
)

type hello struct{ svc.BaseService }

func (hello) Name() string { return "hello" }

func Example() {
	app, err := svc.New(
		svc.Name("hello-app"),
		svc.Version("0.1.0"),
		svc.Commit("dev"),
		svc.Services(hello{}),
		svc.LogWriter(io.Discard),
		svc.HandleSignals(false),
		svc.DrainDelay(0),
		svc.Insecure(),
	)
	if err != nil {
		fmt.Println(err)
		return
	}

	// A real main calls svc.Main(app), which blocks until SIGINT or SIGTERM.
	// Here a Shutdown that waits for Running stops the App.
	go func() {
		for app.Status() != svc.StatusRunning {
			time.Sleep(time.Millisecond)
		}

		_ = app.Shutdown(context.Background())
	}()

	fmt.Println(app.Launch(), app.Status())
	// Output: <nil> Stopped
}
