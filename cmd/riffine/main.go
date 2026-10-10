package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/NLstn/riffine/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "riffine:", err)
		os.Exit(1)
	}
}
