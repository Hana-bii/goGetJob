package file

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gabriel-vasile/mimetype"
)

const DefaultMaxFileSizeBytes int64 = 20 * 1024 * 1024

var defaultExtensions = map[string]struct{}{
	".doc":  {},
	".docx": {},
	".md":   {},
	".pdf":  {},
	".rtf":  {},
	".txt":  {},
}

var defaultMIMEs = map[string]struct{}{
	"application/msword": {},
	"application/pdf":    {},
	"application/rtf":    {},
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": {},
	"text/markdown": {},
	"text/plain":    {},
	"text/rtf":      {},
}

type ValidationOptions struct {
	MaxSizeBytes      int64
	AllowedMIMEs      []string
	AllowedExtensions []string
}

type Validator struct {
	maxSizeBytes int64
	mimes        map[string]struct{}
	extensions   map[string]struct{}
}

func NewValidator(options ValidationOptions) *Validator {
	maxSize := options.MaxSizeBytes
	if maxSize <= 0 {
		maxSize = DefaultMaxFileSizeBytes
	}

	return &Validator{
		maxSizeBytes: maxSize,
		mimes:        stringSet(options.AllowedMIMEs, defaultMIMEs),
		extensions:   extensionSet(options.AllowedExtensions, defaultExtensions),
	}
}

func (v *Validator) Validate(name string, data []byte) error {
	if err := v.ValidateSize(int64(len(data))); err != nil {
		return err
	}
	return v.ValidateType(name, data)
}

func (v *Validator) MaxSizeBytes() int64 {
	if v == nil {
		return DefaultMaxFileSizeBytes
	}
	return v.maxSizeBytes
}

func (v *Validator) ValidateSize(size int64) error {
	if size <= 0 {
		return fmt.Errorf("file is empty")
	}
	if v == nil {
		v = NewValidator(ValidationOptions{})
	}
	if size > v.maxSizeBytes {
		return fmt.Errorf("file size %d exceeds limit %d", size, v.maxSizeBytes)
	}
	return nil
}

func (v *Validator) ValidateType(name string, data []byte) error {
	if v == nil {
		v = NewValidator(ValidationOptions{})
	}

	ext := strings.ToLower(filepath.Ext(name))
	mime := mimetype.Detect(data).String()
	_, extAllowed := v.extensions[ext]
	if ext == "" {
		extAllowed = true
	}
	if _, ok := v.mimes[mime]; ok && extAllowed {
		return nil
	}
	if extAllowed && allowsExtensionFallback(ext, mime) {
		return nil
	}
	return fmt.Errorf("unsupported file type %q with MIME %q", ext, mime)
}

func stringSet(values []string, defaults map[string]struct{}) map[string]struct{} {
	if len(values) == 0 {
		return cloneSet(defaults)
	}

	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.ToLower(value))
		if value != "" {
			set[value] = struct{}{}
		}
	}
	return set
}

func extensionSet(values []string, defaults map[string]struct{}) map[string]struct{} {
	if len(values) == 0 {
		return cloneSet(defaults)
	}

	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			continue
		}
		if !strings.HasPrefix(value, ".") {
			value = "." + value
		}
		set[value] = struct{}{}
	}
	return set
}

func cloneSet(source map[string]struct{}) map[string]struct{} {
	clone := make(map[string]struct{}, len(source))
	for key := range source {
		clone[key] = struct{}{}
	}
	return clone
}

func allowsExtensionFallback(ext, mime string) bool {
	switch ext {
	case ".docx":
		return mime == "application/zip" || mime == "application/octet-stream"
	case ".md", ".rtf", ".txt":
		return strings.HasPrefix(mime, "text/") || mime == "application/octet-stream"
	case ".doc":
		return mime == "application/msword" || mime == "application/octet-stream" || strings.HasPrefix(mime, "text/")
	case ".pdf":
		return mime == "application/pdf" || mime == "application/octet-stream"
	default:
		return false
	}
}
