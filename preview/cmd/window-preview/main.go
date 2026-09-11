package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/viant/agently/preview/window"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	root := flag.String("root", "preview/window/examples/projects", "preview workspace")
	addr := flag.String("addr", "127.0.0.1:8098", "loopback address")
	assets := flag.String("assets", "preview/ui/dist", "built frontend directory")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if e := windowpreview.Serve(ctx, windowpreview.Config{Root: *root, Addr: *addr, Assets: *assets}, os.Stdout); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
