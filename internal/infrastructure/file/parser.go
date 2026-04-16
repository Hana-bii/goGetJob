package file

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	pdf "github.com/ledongthuc/pdf"
)

const (
	DefaultMaxTextBytes             = 5 * 1024 * 1024
	docxXMLDecompressionOverheadMax = int64(64 * 1024)
)

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
	if p == nil {
		p = NewParser(ParserOptions{})
	}
	maxSize := p.validator.MaxSizeBytes()
	readLimit := maxSize
	if readLimit < math.MaxInt64 {
		readLimit++
	}
	data, err := io.ReadAll(io.LimitReader(r, readLimit))
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
		raw = truncateUTF8Bytes(string(data), p.maxTextBytes)
	case ".doc":
		return "", fmt.Errorf("legacy DOC files are not supported")
	case ".docx":
		raw, err = extractDOCX(data, p.maxTextBytes)
	case ".pdf":
		raw, err = extractPDF(data, p.maxTextBytes)
	case ".rtf":
		raw = stripRTF(string(data), p.maxTextBytes)
	default:
		err = fmt.Errorf("unsupported file extension %q", filepath.Ext(name))
	}
	if err != nil {
		return "", err
	}

	return p.cleaner.CleanWithLimit(raw, p.maxTextBytes), nil
}

func extractDOCX(data []byte, maxTextBytes int) (string, error) {
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

	xmlLimit := docxXMLDecompressedByteLimit(maxTextBytes)
	xmlInput := &io.LimitedReader{R: rc, N: xmlLimit + 1}
	out := newLimitedStringBuilder(maxTextBytes)
	decoder := xml.NewDecoder(xmlInput)
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			if xmlInput.N <= 0 {
				return "", fmt.Errorf("docx decompressed XML exceeds %d bytes", xmlLimit)
			}
			return "", fmt.Errorf("parse docx XML: %w", err)
		}

		switch tok := token.(type) {
		case xml.StartElement:
			if tok.Name.Local == "p" && out.Len() > 0 {
				if out.AppendByte('\n') {
					return out.String(), nil
				}
			}
		case xml.CharData:
			if out.WriteBytes(tok) {
				if xmlInput.N <= 0 {
					return "", fmt.Errorf("docx decompressed XML exceeds %d bytes", xmlLimit)
				}
				return out.String(), nil
			}
		}
	}

	if xmlInput.N <= 0 {
		return "", fmt.Errorf("docx decompressed XML exceeds %d bytes", xmlLimit)
	}
	return out.String(), nil
}

func docxXMLDecompressedByteLimit(maxTextBytes int) int64 {
	if maxTextBytes <= 0 {
		maxTextBytes = DefaultMaxTextBytes
	}
	if int64(maxTextBytes) > math.MaxInt64-docxXMLDecompressionOverheadMax-1 {
		return math.MaxInt64 - 1
	}
	return int64(maxTextBytes) + docxXMLDecompressionOverheadMax
}

func extractPDF(data []byte, maxTextBytes int) (string, error) {
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err == nil {
		plain, plainErr := reader.GetPlainText()
		if plainErr == nil {
			text, readErr := readTextWithLimit(plain, maxTextBytes)
			if readErr == nil && strings.TrimSpace(string(text)) != "" {
				return text, nil
			}
		}
	}

	fallback := extractLiteralPDFStrings(data, maxTextBytes)
	if strings.TrimSpace(fallback) != "" {
		return fallback, nil
	}
	if err != nil {
		return "", fmt.Errorf("parse pdf: %w", err)
	}
	return "", fmt.Errorf("parse pdf: no extractable text")
}

var pdfLiteralString = regexp.MustCompile(`\((?:\\.|[^\\)])*\)\s*Tj`)

func extractLiteralPDFStrings(data []byte, maxTextBytes int) string {
	out := newLimitedStringBuilder(maxTextBytes)
	remaining := data
	for len(remaining) > 0 && !out.Capped() {
		loc := pdfLiteralString.FindIndex(remaining)
		if loc == nil {
			break
		}
		match := remaining[loc[0]:loc[1]]
		start := bytes.IndexByte(match, '(')
		end := bytes.LastIndexByte(match, ')')
		if start < 0 || end <= start {
			remaining = remaining[loc[1]:]
			continue
		}
		if out.Len() > 0 && out.AppendByte('\n') {
			break
		}
		if writeDecodedPDFLiteralString(string(match[start+1:end]), out) {
			break
		}
		remaining = remaining[loc[1]:]
	}
	return out.String()
}

func writeDecodedPDFLiteralString(text string, out *limitedStringBuilder) bool {
	for i := 0; i < len(text); i++ {
		if text[i] != '\\' || i == len(text)-1 {
			r, size := utf8.DecodeRuneInString(text[i:])
			if r == utf8.RuneError && size == 1 {
				if text[i] < utf8.RuneSelf {
					if out.AppendByte(text[i]) {
						return true
					}
				}
				continue
			}
			if out.WriteString(text[i : i+size]) {
				return true
			}
			i += size - 1
			continue
		}

		i++
		switch text[i] {
		case 'n':
			if out.AppendByte('\n') {
				return true
			}
		case 'r':
			if out.AppendByte('\r') {
				return true
			}
		case 't':
			if out.AppendByte('\t') {
				return true
			}
		case 'b':
			if out.AppendByte('\b') {
				return true
			}
		case 'f':
			if out.AppendByte('\f') {
				return true
			}
		case '(', ')', '\\':
			if out.AppendByte(text[i]) {
				return true
			}
		default:
			if text[i] >= '0' && text[i] <= '7' {
				end := i + 1
				for end < len(text) && end < i+3 && text[end] >= '0' && text[end] <= '7' {
					end++
				}
				value, err := strconv.ParseInt(text[i:end], 8, 32)
				if err == nil {
					if out.AppendByte(byte(value)) {
						return true
					}
					i = end - 1
					continue
				}
			}
			if out.AppendByte(text[i]) {
				return true
			}
		}
	}
	return false
}

func readTextWithLimit(r io.Reader, limit int) (string, error) {
	if limit <= 0 {
		data, err := io.ReadAll(r)
		return string(data), err
	}

	data, err := io.ReadAll(io.LimitReader(r, int64(limit)+1))
	if err != nil {
		return "", err
	}
	if len(data) > limit {
		data = []byte(truncateUTF8Bytes(string(data), limit))
	}
	return string(data), nil
}

func stripRTF(text string, maxTextBytes int) string {
	out := newLimitedStringBuilder(maxTextBytes)
	for i := 0; i < len(text) && !out.Capped(); {
		switch text[i] {
		case '{', '}':
			i++
		case '\\':
			if strings.HasPrefix(text[i:], `\par`) {
				if out.AppendByte('\n') {
					return out.String()
				}
				i += len(`\par`)
				if i < len(text) && text[i] == ' ' {
					i++
				}
				continue
			}
			if strings.HasPrefix(text[i:], `\'`) {
				i += len(`\'`)
				for skipped := 0; skipped < 2 && i < len(text) && isHex(text[i]); skipped++ {
					i++
				}
				continue
			}

			i++
			for i < len(text) && ((text[i] >= 'a' && text[i] <= 'z') || (text[i] >= 'A' && text[i] <= 'Z')) {
				i++
			}
			if i < len(text) && text[i] == '-' {
				i++
			}
			for i < len(text) && text[i] >= '0' && text[i] <= '9' {
				i++
			}
			if i < len(text) && text[i] == ' ' {
				i++
			}
		default:
			r, size := utf8.DecodeRuneInString(text[i:])
			if r == utf8.RuneError && size == 1 {
				if text[i] < utf8.RuneSelf && out.AppendByte(text[i]) {
					return out.String()
				}
				i++
				continue
			}
			if out.WriteString(text[i : i+size]) {
				return out.String()
			}
			i += size
		}
	}
	return out.String()
}

func isHex(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

type limitedStringBuilder struct {
	builder strings.Builder
	limit   int
	capped  bool
}

func newLimitedStringBuilder(limit int) *limitedStringBuilder {
	return &limitedStringBuilder{limit: limit}
}

func (b *limitedStringBuilder) Len() int {
	return b.builder.Len()
}

func (b *limitedStringBuilder) Capped() bool {
	return b.capped
}

func (b *limitedStringBuilder) String() string {
	return b.builder.String()
}

func (b *limitedStringBuilder) AppendByte(value byte) bool {
	if b.limit <= 0 {
		b.builder.WriteByte(value)
		return false
	}
	if b.builder.Len() >= b.limit {
		b.capped = true
		return true
	}
	b.builder.WriteByte(value)
	b.capped = b.builder.Len() >= b.limit
	return b.capped
}

func (b *limitedStringBuilder) WriteBytes(data []byte) bool {
	return b.WriteString(string(data))
}

func (b *limitedStringBuilder) WriteString(value string) bool {
	if value == "" {
		return b.capped
	}
	if b.limit <= 0 {
		b.builder.WriteString(value)
		return false
	}

	remaining := b.limit - b.builder.Len()
	if remaining <= 0 {
		b.capped = true
		return true
	}
	if len(value) <= remaining {
		b.builder.WriteString(value)
		b.capped = b.builder.Len() >= b.limit
		return b.capped
	}

	b.builder.WriteString(truncateUTF8Bytes(value, remaining))
	b.capped = true
	return true
}
