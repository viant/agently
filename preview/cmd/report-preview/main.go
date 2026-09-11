package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"github.com/viant/agently/preview/report"
	preview "github.com/viant/agently/preview/report/provider"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := reportpreview.RunCLI(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		var d *preview.Diagnostic
		if !errors.As(err, &d) {
			d = &preview.Diagnostic{Code: "mockPreviewMalformedPackage", Message: err.Error(), Path: "report.yaml"}
		}
		_ = json.NewEncoder(os.Stderr).Encode(d)
		os.Exit(1)
	}
}
