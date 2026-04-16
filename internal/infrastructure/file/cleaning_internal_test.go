package file

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTruncateUTF8BytesKeepsContentAfterEarlierInvalidByte(t *testing.T) {
	input := string([]byte{'a', 'b', 'c', 0xff, 'd', 'e', 'f'}) + "界"

	got := truncateUTF8Bytes(input, 7)

	require.Equal(t, string([]byte{'a', 'b', 'c', 0xff, 'd', 'e', 'f'}), got)
}

func TestTruncateUTF8BytesDropsOnlySplitTrailingRune(t *testing.T) {
	require.Equal(t, "abc", truncateUTF8Bytes("abc界z", 5))
	require.Equal(t, "abc界", truncateUTF8Bytes("abc界z", 6))
}
