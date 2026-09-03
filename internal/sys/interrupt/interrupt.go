package interrupt

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var (
	mu sync.Mutex
	ch chan os.Signal
)

// Context returns a copy of parent that is cancelled on SIGINT or SIGTERM.
func Context(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	mu.Lock()
	ch = c
	mu.Unlock()
	go func() {
		select {
		case <-c:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, func() {
		signal.Stop(c)
		cancel()
	}
}

// Lend makes SIGINT and SIGQUIT no-ops for this process until the returned func gives them back.
func Lend() (restore func()) {
	signal.Ignore(os.Interrupt, syscall.SIGQUIT)
	return func() {
		signal.Reset(syscall.SIGQUIT)
		mu.Lock()
		c := ch
		mu.Unlock()
		if c == nil {
			signal.Reset(os.Interrupt)
			return
		}
		signal.Notify(c, os.Interrupt)
	}
}
