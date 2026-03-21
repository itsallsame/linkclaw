package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/relayserver"
)

func main() {
	fs := flag.NewFlagSet("linkclaw-relay-shim", flag.ExitOnError)
	listen := fs.String("listen", "127.0.0.1:8788", "listen address")
	dbPath := fs.String("db", "./linkclaw-relay.db", "sqlite database path")
	fs.Parse(os.Args[1:])

	server, result, err := relayserver.Start(*dbPath, *listen)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start relay shim: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, "linkclaw-relay-shim serving %s\n", result.URL)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-signals:
	case err := <-server.Done():
		if err != nil {
			fmt.Fprintf(os.Stderr, "relay shim stopped: %v\n", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "shutdown relay shim: %v\n", err)
		os.Exit(1)
	}
}
