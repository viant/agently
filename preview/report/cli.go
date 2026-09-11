package reportpreview

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	preview "github.com/viant/agently/preview/report/provider"
)

// RunCLI supports both `serve folder --variant empty` and flags before folder.
func RunCLI(ctx context.Context, args []string, out, errOut io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: report-preview <serve|validate|describe|query|compile|export> <folder> [--variant name] [--datasource id] [--request file] [--parameters file] [--out file] [--addr 127.0.0.1:8095]")
	}
	command := args[0]
	args = args[1:]
	folder := ""
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		folder = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("report-preview "+command, flag.ContinueOnError)
	fs.SetOutput(errOut)
	variant := fs.String("variant", "", "fixture variant")
	datasource := fs.String("datasource", "", "datasource ID")
	request := fs.String("request", "", "normalized query JSON file, or - for stdin")
	paramsFile := fs.String("parameters", "", "report parameters JSON file")
	block := fs.String("block", "", "table block for CSV/XLSX export")
	output := fs.String("out", "", "output file")
	addr := fs.String("addr", "127.0.0.1:8095", "loopback listen address")
	full := fs.Bool("full", false, "compile all matching rows")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if folder == "" && fs.NArg() == 1 {
		folder = fs.Arg(0)
	} else if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	if folder == "" {
		return fmt.Errorf("report folder is required")
	}
	if command == "serve" {
		return Serve(ctx, Config{Folder: folder, Variant: *variant, Addr: *addr}, out)
	}
	p, err := preview.Load(folder, *variant)
	if err != nil {
		return err
	}
	params := preview.Object{}
	if *paramsFile != "" {
		b, e := os.ReadFile(*paramsFile)
		if e != nil {
			return e
		}
		if e = json.Unmarshal(b, &params); e != nil {
			return e
		}
	}
	if command == "query" || command == "compile" || command == "export" {
		close, err := connectCLI(ctx, p, folder, *variant)
		if err != nil {
			return err
		}
		defer close()
	}
	var value any
	switch command {
	case "validate":
		c, e := p.Compile(params, false)
		if e != nil {
			return e
		}
		value = preview.Object{"valid": true, "variant": p.Variant, "warnings": c.Warnings, "datasets": len(c.Results)}
	case "describe":
		value, err = p.DescribeDataSource(*datasource)
	case "query":
		if *request == "" {
			return fmt.Errorf("--request is required")
		}
		var r io.Reader = os.Stdin
		if *request != "-" {
			f, e := os.Open(*request)
			if e != nil {
				return e
			}
			defer f.Close()
			r = f
		}
		q, e := DecodeQuery(r)
		if e != nil {
			return e
		}
		if *datasource == "" {
			*datasource = q.DataSource
		}
		value, err = p.Execute(*datasource, q)
	case "compile":
		value, err = p.Compile(params, *full)
	case "export":
		if *output == "" {
			return fmt.Errorf("export requires --out file.pdf, file.html, file.csv or file.xlsx")
		}
		c, e := p.Compile(params, true)
		if e != nil {
			return e
		}
		b, e := c.Export(strings.TrimPrefix(filepath.Ext(*output), "."), *block)
		if e != nil {
			return e
		}
		if e = os.WriteFile(*output, b, 0644); e != nil {
			return e
		}
		fmt.Fprintln(out, *output)
		return nil
	default:
		return fmt.Errorf("unknown command %q", command)
	}
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if *output != "" {
		return os.WriteFile(*output, b, 0644)
	}
	_, err = out.Write(b)
	return err
}
