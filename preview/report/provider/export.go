package preview

import (
	"fmt"
	"github.com/viant/forge/backend/reporting/export/csv"
	"github.com/viant/forge/backend/reporting/export/xlsx"
	reportfill "github.com/viant/forge/backend/reporting/fill"
)

// Export uses Forge's existing exporters. CSV and XLSX select one table block,
// matching their existing public contract rather than inventing a new layout.
func (c *Compiled) Export(format, blockID string) ([]byte, error) {
	switch format {
	case "pdf":
		return c.PDF()
	case "html":
		return c.HTML()
	case "csv", "xlsx":
	default:
		return nil, fmt.Errorf("unsupported export format %q", format)
	}
	fill, err := reportfill.DecodeJSON(c.ReportFill)
	if err != nil {
		return nil, err
	}
	selected := []reportfill.Block{}
	for _, b := range fill.Blocks {
		if b.Kind == "tableBlock" && (blockID == "" || b.ID == blockID) {
			selected = append(selected, b)
		}
	}
	if len(selected) != 1 {
		return nil, fmt.Errorf("%s export requires --block selecting one table; matched %d", format, len(selected))
	}
	fill.Blocks = selected
	if format == "csv" {
		return csv.Render(fill)
	}
	return xlsx.Render(fill)
}
