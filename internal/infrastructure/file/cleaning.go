package file

import (
	"html"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

type TextCleaner struct{}

var (
	imageFilenameLine = regexp.MustCompile(`(?i)^\s*[\w\- ./\\]+\.(png|jpe?g|gif|bmp|webp|svg)\s*$`)
	imageURLLine      = regexp.MustCompile(`(?i)^\s*https?://\S+\.(png|jpe?g|gif|bmp|webp|svg)(\?\S*)?\s*$`)
	fileURLLine       = regexp.MustCompile(`(?i)^\s*file:\S+\s*$`)
	separatorLine     = regexp.MustCompile(`^\s*[-=_*]{3,}\s*$`)
	htmlTag           = regexp.MustCompile(`(?s)<[^>]+>`)
	newlineRun        = regexp.MustCompile(`\n{3,}`)
	spaceRun          = regexp.MustCompile(`[ \t\n\r]+`)
)

func NewTextCleaner() TextCleaner {
	return TextCleaner{}
}

func (TextCleaner) Clean(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = removeDisallowedControlChars(text)

	lines := strings.Split(text, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRightFunc(line, unicode.IsSpace)
		if shouldDropLine(line) {
			continue
		}
		kept = append(kept, line)
	}

	text = strings.Join(kept, "\n")
	text = newlineRun.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

func (c TextCleaner) CleanWithLimit(text string, limit int) string {
	cleaned := c.Clean(text)
	if limit <= 0 || utf8.RuneCountInString(cleaned) <= limit {
		return cleaned
	}

	runes := []rune(cleaned)
	return strings.TrimSpace(string(runes[:limit]))
}

func (c TextCleaner) CleanToSingleLine(text string) string {
	cleaned := c.Clean(text)
	return strings.TrimSpace(spaceRun.ReplaceAllString(cleaned, " "))
}

func (TextCleaner) StripHTML(text string) string {
	text = htmlTag.ReplaceAllString(text, " ")
	text = html.UnescapeString(text)
	return strings.TrimSpace(spaceRun.ReplaceAllString(text, " "))
}

func removeDisallowedControlChars(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, text)
}

func shouldDropLine(line string) bool {
	return imageFilenameLine.MatchString(line) ||
		imageURLLine.MatchString(line) ||
		fileURLLine.MatchString(line) ||
		separatorLine.MatchString(line)
}
