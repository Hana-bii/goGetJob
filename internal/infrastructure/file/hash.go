package file

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
)

const hashBufferSize = 8192

func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func HashReader(r io.Reader) (string, error) {
	if r == nil {
		return "", fmt.Errorf("reader is required")
	}

	h := sha256.New()
	buf := make([]byte, hashBufferSize)
	if _, err := io.CopyBuffer(h, r, buf); err != nil {
		return "", fmt.Errorf("hash reader: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
