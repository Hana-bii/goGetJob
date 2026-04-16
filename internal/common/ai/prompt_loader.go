package ai

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var templateVariablePattern = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)

type PromptLoader struct {
	root string
}

func NewPromptLoader(root string) *PromptLoader {
	if root == "" {
		root = "internal/prompts"
	}
	if absRoot, err := filepath.Abs(root); err == nil {
		root = absRoot
	}
	return &PromptLoader{root: root}
}

func (l *PromptLoader) Load(name string) (string, error) {
	path := filepath.Clean(filepath.Join(l.root, name))
	root := filepath.Clean(l.root)
	if !isWithin(path, root) {
		return "", fmt.Errorf("prompt not found %q: path escapes prompt root", name)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("prompt not found %q: %w", name, err)
		}
		return "", fmt.Errorf("read prompt %q: %w", name, err)
	}
	return string(raw), nil
}

func (l *PromptLoader) Render(name string, values map[string]string) (string, error) {
	content, err := l.Load(name)
	if err != nil {
		return "", err
	}
	return RenderTemplate(content, values), nil
}

func RenderTemplate(content string, values map[string]string) string {
	return templateVariablePattern.ReplaceAllStringFunc(content, func(match string) string {
		key := templateVariablePattern.FindStringSubmatch(match)[1]
		if value, ok := values[key]; ok {
			return value
		}
		return match
	})
}

func isWithin(path, root string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !filepath.IsAbs(rel) && !startsWithDotDot(rel))
}

func startsWithDotDot(path string) bool {
	return path == ".." || len(path) > 3 && path[:3] == ".."+string(filepath.Separator)
}
