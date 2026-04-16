package export

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/signintech/gopdf"
)

type PDFExporter interface {
	ExportReport(ctx context.Context, report Report) ([]byte, error)
}

type PDFOptions struct {
	FontPath   string
	FontBytes  []byte
	FontFamily string
}

type Report struct {
	Title    string
	Sections []ReportSection
}

type ReportSection struct {
	Heading string
	Body    string
}

type pdfExporter struct {
	options PDFOptions
}

func NewPDFExporter(options PDFOptions) PDFExporter {
	return &pdfExporter{options: options}
}

func (e *pdfExporter) ExportReport(ctx context.Context, report Report) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if e == nil {
		e = &pdfExporter{}
	}

	if e.options.FontPath != "" || len(e.options.FontBytes) > 0 {
		return e.exportWithTTF(report)
	}
	if err := validateASCIIReport(report); err != nil {
		return nil, err
	}
	return exportSimplePDF(report), nil
}

func (e *pdfExporter) exportWithTTF(report Report) ([]byte, error) {
	family := e.options.FontFamily
	if family == "" {
		family = "report-font"
	}

	var pdf gopdf.GoPdf
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	pdf.SetMargins(48, 48, 48, 48)
	pdf.AddPage()

	if e.options.FontPath != "" {
		if err := pdf.AddTTFFont(family, e.options.FontPath); err != nil {
			return nil, fmt.Errorf("load font: %w", err)
		}
	} else if len(e.options.FontBytes) > 0 {
		if err := pdf.AddTTFFontData(family, e.options.FontBytes); err != nil {
			return nil, fmt.Errorf("load font data: %w", err)
		}
	}

	if err := pdf.SetFont(family, "", 16); err != nil {
		return nil, fmt.Errorf("set title font: %w", err)
	}
	if err := pdf.Text(nonEmpty(report.Title, "Report")); err != nil {
		return nil, fmt.Errorf("write title: %w", err)
	}
	pdf.Br(28)

	for _, section := range report.Sections {
		if err := pdf.SetFont(family, "", 12); err != nil {
			return nil, fmt.Errorf("set heading font: %w", err)
		}
		if section.Heading != "" {
			if err := pdf.Text(section.Heading); err != nil {
				return nil, fmt.Errorf("write heading: %w", err)
			}
			pdf.Br(18)
		}

		if err := pdf.SetFont(family, "", 10); err != nil {
			return nil, fmt.Errorf("set body font: %w", err)
		}
		if strings.TrimSpace(section.Body) != "" {
			if err := pdf.MultiCell(&gopdf.Rect{W: 500, H: 14}, section.Body); err != nil {
				return nil, fmt.Errorf("write body: %w", err)
			}
		}
		pdf.Br(10)
	}

	out, err := pdf.GetBytesPdfReturnErr()
	if err != nil {
		return nil, fmt.Errorf("render pdf: %w", err)
	}
	return out, nil
}

func exportSimplePDF(report Report) []byte {
	lines := reportLines(report)
	stream := simplePDFContentStream(lines)

	objects := []string{
		"1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj\n",
		"2 0 obj << /Type /Pages /Kids [3 0 R] /Count 1 >> endobj\n",
		"3 0 obj << /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >> endobj\n",
		"4 0 obj << /Type /Font /Subtype /Type1 /BaseFont /Helvetica >> endobj\n",
		"5 0 obj << /Length " + strconv.Itoa(len(stream)) + " >> stream\n" + stream + "\nendstream endobj\n",
	}

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, 0, len(objects))
	for _, obj := range objects {
		offsets = append(offsets, buf.Len())
		buf.WriteString(obj)
	}

	xref := buf.Len()
	buf.WriteString("xref\n0 6\n0000000000 65535 f \n")
	for _, offset := range offsets {
		buf.WriteString(fmt.Sprintf("%010d 00000 n \n", offset))
	}
	buf.WriteString("trailer << /Root 1 0 R /Size 6 >>\nstartxref\n")
	buf.WriteString(strconv.Itoa(xref))
	buf.WriteString("\n%%EOF")
	return buf.Bytes()
}

func reportLines(report Report) []string {
	lines := []string{nonEmpty(report.Title, "Report"), ""}
	for _, section := range report.Sections {
		if strings.TrimSpace(section.Heading) != "" {
			lines = append(lines, section.Heading)
		}
		for _, line := range wrapASCII(section.Body, 88) {
			lines = append(lines, line)
		}
		lines = append(lines, "")
	}
	return lines
}

func simplePDFContentStream(lines []string) string {
	var stream strings.Builder
	stream.WriteString("BT\n/F1 12 Tf\n72 780 Td\n14 TL\n")
	for _, line := range lines {
		stream.WriteString("(")
		stream.WriteString(escapePDFText(line))
		stream.WriteString(") Tj\nT*\n")
	}
	stream.WriteString("ET")
	return stream.String()
}

func wrapASCII(text string, width int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	words := strings.Fields(text)
	var lines []string
	var current strings.Builder
	for _, word := range words {
		if current.Len() == 0 {
			current.WriteString(word)
			continue
		}
		if current.Len()+1+len(word) > width {
			lines = append(lines, current.String())
			current.Reset()
			current.WriteString(word)
			continue
		}
		current.WriteByte(' ')
		current.WriteString(word)
	}
	if current.Len() > 0 {
		lines = append(lines, current.String())
	}
	return lines
}

func escapePDFText(text string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`, "\n", " ", "\r", " ", "\t", " ")
	return replacer.Replace(text)
}

func validateASCIIReport(report Report) error {
	if !isASCII(report.Title) {
		return simplePDFASCIIError("title")
	}
	for _, section := range report.Sections {
		if !isASCII(section.Heading) {
			return simplePDFASCIIError("section heading")
		}
		if !isASCII(section.Body) {
			return simplePDFASCIIError("section body")
		}
	}
	return nil
}

func simplePDFASCIIError(field string) error {
	return fmt.Errorf("simple PDF exporter supports ASCII only in %s; provide FontPath or FontBytes for UTF-8 content", field)
}

func isASCII(text string) bool {
	for _, r := range text {
		if r > 127 {
			return false
		}
	}
	return true
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
