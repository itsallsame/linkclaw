package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/xiewanpeng/claw-identity/internal/registry"
)

func main() {
	fs := flag.NewFlagSet("linkclaw-relay", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:8940", "listen address")
	dbPath := fs.String("db", "./linkclaw-registry.db", "registry sqlite database path")
	fs.Parse(os.Args[1:])

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	service, err := registry.Open(ctx, *dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open registry: %v\n", err)
		os.Exit(1)
	}
	defer service.Close()

	fmt.Fprintf(os.Stdout, "linkclaw registry listening on http://%s\n", *addr)
	if err := registry.Serve(ctx, *addr, service); err != nil {
		fmt.Fprintf(os.Stderr, "serve registry: %v\n", err)
		os.Exit(1)
	}
}
