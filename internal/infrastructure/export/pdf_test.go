package export_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"goGetJob/internal/infrastructure/export"
)

func TestPDFExporterEmitsPDFBytesForSimpleReport(t *testing.T) {
	exporter := export.NewPDFExporter(export.PDFOptions{})
	report := export.Report{
		Title: "Interview Report",
		Sections: []export.ReportSection{
			{Heading: "Summary", Body: "Candidate is ready for backend interviews."},
			{Heading: "Next Steps", Body: "Practice Redis and system design."},
		},
	}

	got, err := exporter.ExportReport(context.Background(), report)

	require.NoError(t, err)
	require.True(t, len(got) > 0)
	require.Equal(t, "%PDF", string(got[:4]))
}

func TestPDFExporterAcceptsFontPathOptionForUTF8Reports(t *testing.T) {
	exporter := export.NewPDFExporter(export.PDFOptions{FontPath: "missing-font.ttf"})
	report := export.Report{
		Title: "中文报告",
		Sections: []export.ReportSection{
			{Heading: "总结", Body: "支持通过 TTF 字体路径渲染 UTF-8 内容。"},
		},
	}

	_, err := exporter.ExportReport(context.Background(), report)

	require.ErrorContains(t, err, "font")
}
