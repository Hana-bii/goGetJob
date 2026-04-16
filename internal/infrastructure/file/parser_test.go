package file_test

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	docfile "goGetJob/internal/infrastructure/file"
)

func TestHashBytesReturnsStableLowercaseSHA256(t *testing.T) {
	got := docfile.HashBytes([]byte("resume text"))

	require.Equal(t, "a8722f53a5b1d3ed2e23f2ecddc927aa6015a3da6274a3ae4754c40aa232e13f", got)
}

func TestHashReaderReturnsStableLowercaseSHA256(t *testing.T) {
	got, err := docfile.HashReader(strings.NewReader("resume text"))

	require.NoError(t, err)
	require.Equal(t, "a8722f53a5b1d3ed2e23f2ecddc927aa6015a3da6274a3ae4754c40aa232e13f", got)
}

func TestValidatorAcceptsSupportedMIMEAndExtensionFallback(t *testing.T) {
	validator := docfile.NewValidator(docfile.ValidationOptions{MaxSizeBytes: 16})

	err := validator.Validate("notes.md", []byte("# hello"))

	require.NoError(t, err)
}

func TestValidatorRejectsEmptyOversizedAndUnsupportedFiles(t *testing.T) {
	validator := docfile.NewValidator(docfile.ValidationOptions{MaxSizeBytes: 4})

	require.ErrorContains(t, validator.Validate("empty.txt", nil), "empty")
	require.ErrorContains(t, validator.Validate("big.txt", []byte("12345")), "exceeds")
	require.ErrorContains(t, validator.Validate("legacy.exe", []byte("MZ")), "unsupported")
	require.ErrorContains(t, docfile.NewValidator(docfile.ValidationOptions{MaxSizeBytes: 64}).Validate("plain.exe", []byte("# hello\nthis is plain text\n")), "unsupported")
	require.ErrorContains(t, docfile.NewValidator(docfile.ValidationOptions{MaxSizeBytes: 2048}).Validate("renamed.exe", simplePDFWithText("pdf content")), "unsupported")
}

func TestParserExtractsAndCleansTXTAndMarkdown(t *testing.T) {
	parser := docfile.NewParser(docfile.ParserOptions{})
	input := []byte("Title\r\nimage.png\r\nhttps://example.com/photo.jpg\r\n---\r\n正文\tline  \r\n\r\n\r\nnext")

	got, err := parser.ParseBytes(context.Background(), "notes.md", input)

	require.NoError(t, err)
	require.Equal(t, "Title\n正文\tline\n\nnext", got)

	got, err = parser.ParseBytes(context.Background(), "notes.txt", []byte("plain text"))

	require.NoError(t, err)
	require.Equal(t, "plain text", got)
}

func TestParserExtractsDOCXText(t *testing.T) {
	parser := docfile.NewParser(docfile.ParserOptions{})
	data := buildDOCX(t, []string{"第一段 resume", "second paragraph"})

	got, err := parser.ParseBytes(context.Background(), "resume.docx", data)

	require.NoError(t, err)
	require.Contains(t, got, "第一段 resume")
	require.Contains(t, got, "second paragraph")
}

func TestParserExtractsPDFText(t *testing.T) {
	parser := docfile.NewParser(docfile.ParserOptions{})

	got, err := parser.ParseBytes(context.Background(), "resume.pdf", simplePDFWithText("PDF resume text"))

	require.NoError(t, err)
	require.Contains(t, got, "PDF resume text")
}

func TestParserRejectsLegacyDOC(t *testing.T) {
	parser := docfile.NewParser(docfile.ParserOptions{})

	_, err := parser.ParseBytes(context.Background(), "resume.doc", []byte("legacy binary"))

	require.ErrorContains(t, err, "legacy DOC")
}

func TestCleanWithLimitAndSingleLineAndStripHTML(t *testing.T) {
	cleaner := docfile.NewTextCleaner()

	require.Equal(t, "hello", cleaner.CleanWithLimit("hello world", 5))
	require.Equal(t, "hello world", cleaner.CleanToSingleLine("hello\n\tworld"))
	require.Equal(t, "Title & Body", cleaner.StripHTML("<h1>Title</h1><p>&amp; Body</p>"))
}

func buildDOCX(t *testing.T, paragraphs []string) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("word/document.xml")
	require.NoError(t, err)

	var xml strings.Builder
	xml.WriteString(`<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	for _, paragraph := range paragraphs {
		xml.WriteString("<w:p><w:r><w:t>")
		xml.WriteString(paragraph)
		xml.WriteString("</w:t></w:r></w:p>")
	}
	xml.WriteString("</w:body></w:document>")
	_, err = w.Write([]byte(xml.String()))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	return buf.Bytes()
}

func simplePDFWithText(text string) []byte {
	stream := "BT /F1 12 Tf 72 720 Td (" + text + ") Tj ET"
	objects := []string{
		"1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj\n",
		"2 0 obj << /Type /Pages /Kids [3 0 R] /Count 1 >> endobj\n",
		"3 0 obj << /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >> endobj\n",
		"4 0 obj << /Type /Font /Subtype /Type1 /BaseFont /Helvetica >> endobj\n",
		"5 0 obj << /Length " + strconv.Itoa(len(stream)) + " >> stream\n" + stream + "\nendstream endobj\n",
	}

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for _, obj := range objects {
		offsets = append(offsets, buf.Len())
		buf.WriteString(obj)
	}
	xref := buf.Len()
	buf.WriteString("xref\n0 6\n0000000000 65535 f \n")
	for _, offset := range offsets[1:] {
		buf.WriteString(fmt.Sprintf("%010d 00000 n \n", offset))
	}
	buf.WriteString("trailer << /Root 1 0 R /Size 6 >>\nstartxref\n")
	buf.WriteString(strconv.Itoa(xref))
	buf.WriteString("\n%%EOF")
	return buf.Bytes()
}
