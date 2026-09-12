package agently

import (
	"context"
	"fmt"
	"os"

	reportpreview "github.com/viant/agently/preview/report"
)

// ReportPreviewCmd exposes the generic report preview through the main Agently
// executable. Product-owned report selectors and MCP arguments remain data.
type ReportPreviewCmd struct {
	ReportRoot string `long:"report-root" description:"Forge report catalog directory"`
	GroupID    string `long:"group-id" description:"report group identity"`
	ReportID   string `long:"report-id" description:"report identity to preview"`
	MCPURL     string `long:"mcp-url" description:"optional MCP endpoint override"`
	Variant    string `long:"variant" description:"fixture variant"`
	Datasource string `long:"datasource" description:"datasource ID"`
	Request    string `long:"request" description:"normalized query JSON file, or - for stdin"`
	Parameters string `long:"parameters" description:"report parameters JSON file"`
	Block      string `long:"block" description:"table block for CSV/XLSX export"`
	Out        string `long:"out" description:"output file"`
	Addr       string `long:"addr" description:"loopback listen address" default:"127.0.0.1:8095"`
	Assets     string `long:"assets" description:"built frontend directory" default:"preview/ui/dist"`
	Full       bool   `long:"full" description:"compile all matching rows"`
}

func (c *ReportPreviewCmd) Execute(args []string) error {
	if len(args) < 1 || len(args) > 2 || (args[0] != "list" && len(args) != 2) {
		return fmt.Errorf("usage: agently report-preview <list|serve|validate|describe|query|compile|export> [fixture-folder] [options]")
	}
	forward := append([]string{}, args...)
	appendValue := func(name, value string) {
		if value != "" {
			forward = append(forward, name, value)
		}
	}
	appendValue("--report-root", c.ReportRoot)
	appendValue("--group-id", c.GroupID)
	appendValue("--report-id", c.ReportID)
	appendValue("--mcp-url", c.MCPURL)
	appendValue("--variant", c.Variant)
	appendValue("--datasource", c.Datasource)
	appendValue("--request", c.Request)
	appendValue("--parameters", c.Parameters)
	appendValue("--block", c.Block)
	appendValue("--out", c.Out)
	appendValue("--addr", c.Addr)
	appendValue("--assets", c.Assets)
	if c.Full {
		forward = append(forward, "--full")
	}
	return reportpreview.RunCLI(context.Background(), forward, os.Stdout, os.Stderr)
}
