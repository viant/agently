package gobootstrap

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/google/gops/agent"
)

var gopsOnce sync.Once

// EnableDiagnostics starts a gops agent when AGENTLY_ENABLE_GOPS is set.
// It is safe to call multiple times in a process.
func EnableDiagnostics() {
	gopsOnce.Do(func() {
		if !envEnabled("AGENTLY_ENABLE_GOPS") {
			return
		}
		opts := agent.Options{
			Addr:            strings.TrimSpace(os.Getenv("AGENTLY_GOPS_ADDR")),
			ConfigDir:       strings.TrimSpace(os.Getenv("AGENTLY_GOPS_CONFIG_DIR")),
			ShutdownCleanup: true,
		}
		if err := agent.Listen(opts); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "gops listen failed pid=%d err=%v\n", os.Getpid(), err)
			return
		}
		_, _ = fmt.Fprintf(os.Stderr, "gops listening pid=%d addr=%q config_dir=%q\n", os.Getpid(), opts.Addr, opts.ConfigDir)
	})
}

func envEnabled(key string) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return false
	}
	enabled, err := strconv.ParseBool(value)
	if err != nil {
		return false
	}
	return enabled
}
