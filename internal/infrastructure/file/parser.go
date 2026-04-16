package file

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	pdf "github.com/ledongthuc/pdf"
)

const DefaultMaxTextBytes = 5 * 1024 * 1024

type ParserOptions struct {
	MaxTextBytes int
	Validation   ValidationOptions
	Cleaner      TextCleaner
}

type Parser struct {
	maxTextBytes int
	validator    *Validator
	cleaner      TextCleaner
}

func NewParser(options ParserOptions) *Parser {
	maxTextBytes := options.MaxTextBytes
	if maxTextBytes <= 0 {
		maxTextBytes = DefaultMaxTextBytes
	}

	cleaner := options.Cleaner
	if cleaner == (TextCleaner{}) {
		cleaner = NewTextCleaner()
	}

	return &Parser{
		maxTextBytes: maxTextBytes,
		validator:    NewValidator(options.Validation),
		cleaner:      cleaner,
	}
}

func (p *Parser) ParseReader(ctx context.Context, name string, r io.Reader) (string, error) {
	if r == nil {
		return "", fmt.Errorf("reader is required")
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read document: %w", err)
	}
	return p.ParseBytes(ctx, name, data)
}

func (p *Parser) ParseBytes(ctx context.Context, name string, data []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if p == nil {
		p = NewParser(ParserOptions{})
	}
	if err := p.validator.Validate(name, data); err != nil {
		return "", err
	}

	var (
		raw string
		err error
	)

	switch strings.ToLower(filepath.Ext(name)) {
	case ".txt", ".md":
		raw = string(data)
	case ".doc":
		return "", fmt.Errorf("legacy DOC files are not supported")
	case ".docx":
		raw, err = extractDOCX(data)
	case ".pdf":
		raw, err = extractPDF(data)
	case ".rtf":
		raw = stripRTF(string(data))
	default:
		err = fmt.Errorf("unsupported file extension %q", filepath.Ext(name))
	}
	if err != nil {
		return "", err
	}

	return p.cleaner.CleanWithLimit(raw, p.maxTextBytes), nil
}

func extractDOCX(data []byte) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("open docx archive: %w", err)
	}

	var document *zip.File
	for _, file := range reader.File {
		if file.Name == "word/document.xml" {
			document = file
			break
		}
	}
	if document == nil {
		return "", fmt.Errorf("docx missing word/document.xml")
	}

	rc, err := document.Open()
	if err != nil {
		return "", fmt.Errorf("open docx document: %w", err)
	}
	defer rc.Close()

	var out strings.Builder
	decoder := xml.NewDecoder(rc)
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("parse docx XML: %w", err)
		}

		switch tok := token.(type) {
		case xml.StartElement:
			if tok.Name.Local == "p" && out.Len() > 0 {
				out.WriteByte('\n')
			}
		case xml.CharData:
			out.Write([]byte(tok))
		}
	}

	return out.String(), nil
}

func extractPDF(data []byte) (string, error) {
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err == nil {
		plain, plainErr := reader.GetPlainText()
		if plainErr == nil {
			text, readErr := io.ReadAll(plain)
			if readErr == nil && strings.TrimSpace(string(text)) != "" {
				return string(text), nil
			}
		}
	}

	fallback := extractLiteralPDFStrings(data)
	if strings.TrimSpace(fallback) != "" {
		return fallback, nil
	}
	if err != nil {
		return "", fmt.Errorf("parse pdf: %w", err)
	}
	return "", fmt.Errorf("parse pdf: no extractable text")
}

var pdfLiteralString = regexp.MustCompile(`\((?:\\.|[^\\)])*\)\s*Tj`)

func extractLiteralPDFStrings(data []byte) string {
	matches := pdfLiteralString.FindAll(data, -1)
	lines := make([]string, 0, len(matches))
	for _, match := range matches {
		start := bytes.IndexByte(match, '(')
		end := bytes.LastIndexByte(match, ')')
		if start < 0 || end <= start {
			continue
		}
		lines = append(lines, decodePDFLiteralString(string(match[start+1:end])))
	}
	return strings.Join(lines, "\n")
}

func decodePDFLiteralString(text string) string {
	var out strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] != '\\' || i == len(text)-1 {
			out.WriteByte(text[i])
			continue
		}

		i++
		switch text[i] {
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 't':
			out.WriteByte('\t')
		case 'b':
			out.WriteByte('\b')
		case 'f':
			out.WriteByte('\f')
		case '(', ')', '\\':
			out.WriteByte(text[i])
		default:
			if text[i] >= '0' && text[i] <= '7' {
				end := i + 1
				for end < len(text) && end < i+3 && text[end] >= '0' && text[end] <= '7' {
					end++
				}
				value, err := strconv.ParseInt(text[i:end], 8, 32)
				if err == nil {
					out.WriteByte(byte(value))
					i = end - 1
					continue
				}
			}
			out.WriteByte(text[i])
		}
	}
	return out.String()
}

var (
	rtfCommand = regexp.MustCompile(`\\[a-zA-Z]+\d* ?`)
	rtfEscapes = strings.NewReplacer(`\par`, "\n", `\'`, "")
)

func stripRTF(text string) string {
	text = rtfEscapes.Replace(text)
	text = strings.NewReplacer("{", "", "}", "").Replace(text)
	return rtfCommand.ReplaceAllString(text, "")
}
