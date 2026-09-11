package preview

import (
	"encoding/base64"
	"fmt"
	"html"
	"math"
	"regexp"
	"strings"

	reportprint "github.com/viant/forge/backend/reporting/print"
)

// HTML presents the canonical ReportPrint pages without depending on a browser PDF
// plugin. Values, pagination, and charts are the same compiled output used by PDF.
func (c *Compiled) HTML() ([]byte, error) {
	report, err := reportprint.DecodeJSON(c.ReportPrint)
	if err != nil {
		return nil, err
	}
	var out strings.Builder
	out.WriteString(`<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>` + html.EscapeString(report.Title) + `</title><style>body{margin:0;padding:12px;background:#e9edf3;font-family:Arial,sans-serif}.page{background:white;margin:0 auto 16px;box-shadow:0 1px 5px #b5bfcd;width:100%;max-width:1000px}.page svg{display:block;width:100%;height:auto}@media print{body{padding:0;background:white}.page{box-shadow:none;break-after:page;margin:0}}</style></head><body>`)
	for _, page := range report.Pages {
		fmt.Fprintf(&out, `<div class="page"><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %g %g" role="img" aria-label="Page %d of %s">`, report.PageGeometry.Width, report.PageGeometry.Height, page.Number, html.EscapeString(report.Title))
		elements := append([]reportprint.Element{}, page.HeaderElements...)
		elements = append(elements, page.Elements...)
		elements = append(elements, page.FooterElements...)
		for _, e := range elements {
			if err := htmlElement(&out, e); err != nil {
				return nil, err
			}
		}
		out.WriteString(`</svg></div>`)
	}
	out.WriteString(`</body></html>`)
	return []byte(out.String()), nil
}

var colorPattern = regexp.MustCompile(`^(#[0-9a-fA-F]{3,8}|[a-zA-Z]+)$`)

func color(v, fallback string) string {
	if colorPattern.MatchString(v) {
		return v
	}
	return fallback
}
func htmlElement(out *strings.Builder, e reportprint.Element) error {
	b := e.Box
	rect := func(fill, stroke string) {
		fmt.Fprintf(out, `<rect x="%g" y="%g" width="%g" height="%g" rx="%g" fill="%s" stroke="%s" stroke-width="%g"/>`, b.X, b.Y, b.Width, b.Height, e.Radius, color(fill, "none"), color(stroke, "none"), e.StrokeWidth)
	}
	text := func(value string) {
		size := e.FontSize
		if size == 0 {
			size = 10
		}
		x := b.X
		anchor := "start"
		if e.Align == "right" {
			x += b.Width
			anchor = "end"
		} else if e.Align == "center" {
			x += b.Width / 2
			anchor = "middle"
		}
		weight := "400"
		if e.FontWeight == "bold" || e.FontWeight == "600" || e.FontWeight == "700" {
			weight = "700"
		}
		lines := strings.Split(value, "\n")
		for i, line := range lines {
			fmt.Fprintf(out, `<text x="%g" y="%g" font-family="Arial,sans-serif" font-size="%g" font-weight="%s" text-anchor="%s" fill="%s">%s</text>`, x, b.Y+size+float64(i)*size*1.2, size, weight, anchor, color(e.Color, "#101828"), html.EscapeString(line))
		}
	}
	switch e.Kind {
	case "rect":
		rect(e.FillColor, e.StrokeColor)
	case "line":
		fmt.Fprintf(out, `<line x1="%g" y1="%g" x2="%g" y2="%g" stroke="%s" stroke-width="%g"/>`, b.X, b.Y, b.X+b.Width, b.Y+b.Height, color(e.StrokeColor, "#d0d5dd"), math.Max(e.StrokeWidth, 0.5))
	case "text", "tableCellText":
		text(e.Text)
	case "svg":
		fmt.Fprintf(out, `<image x="%g" y="%g" width="%g" height="%g" href="data:image/svg+xml;base64,%s"/>`, b.X, b.Y, b.Width, b.Height, base64.StdEncoding.EncodeToString([]byte(e.SVG)))
	case "image":
		if e.Image == nil || !contains([]string{"image/png", "image/jpeg"}, e.Image.MimeType) {
			return fmt.Errorf("unsupported report image")
		}
		fmt.Fprintf(out, `<image x="%g" y="%g" width="%g" height="%g" href="data:%s;base64,%s"/>`, b.X, b.Y, b.Width, b.Height, e.Image.MimeType, html.EscapeString(e.Image.Payload))
	case "tableCellTone":
		rect(e.BackgroundColor, e.BorderColor)
		text(e.Text)
	case "tableCellBadge":
		rect(e.BackgroundColor, e.BorderColor)
		text(e.Label)
	case "tableCellDataBar":
		rect(e.BackgroundColor, e.BorderColor)
		fraction := 0.0
		if e.Max > e.Min {
			fraction = math.Max(0, math.Min(1, (e.Value-e.Min)/(e.Max-e.Min)))
		}
		width := b.Width
		b.Width *= fraction
		rect(e.FillColor, "")
		b.Width = width
		text(e.Text)
	default:
		return fmt.Errorf("unsupported ReportPrint element %q", e.Kind)
	}
	return nil
}
