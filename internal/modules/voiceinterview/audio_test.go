package voiceinterview

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPCMToWAVAddsRIFFHeaderAndPreservesPCM(t *testing.T) {
	pcm := []byte{0x01, 0x02, 0x03, 0x04}

	wav, err := PCMToWAV(pcm, 16000, 1, 16)

	require.NoError(t, err)
	require.Len(t, wav, 44+len(pcm))
	require.Equal(t, []byte("RIFF"), wav[0:4])
	require.Equal(t, []byte("WAVE"), wav[8:12])
	require.Equal(t, []byte("fmt "), wav[12:16])
	require.Equal(t, uint32(36+len(pcm)), binary.LittleEndian.Uint32(wav[4:8]))
	require.Equal(t, uint16(1), binary.LittleEndian.Uint16(wav[20:22]))
	require.Equal(t, uint16(1), binary.LittleEndian.Uint16(wav[22:24]))
	require.Equal(t, uint32(16000), binary.LittleEndian.Uint32(wav[24:28]))
	require.Equal(t, uint32(32000), binary.LittleEndian.Uint32(wav[28:32]))
	require.Equal(t, uint16(2), binary.LittleEndian.Uint16(wav[32:34]))
	require.Equal(t, uint16(16), binary.LittleEndian.Uint16(wav[34:36]))
	require.Equal(t, []byte("data"), wav[36:40])
	require.Equal(t, uint32(len(pcm)), binary.LittleEndian.Uint32(wav[40:44]))
	require.True(t, bytes.Equal(pcm, wav[44:]))
}

func TestPCMToWAVRejectsInvalidAudioParameters(t *testing.T) {
	_, err := PCMToWAV([]byte{1, 2}, 0, 1, 16)
	require.Error(t, err)

	_, err = PCMToWAV([]byte{1, 2}, 16000, 0, 16)
	require.Error(t, err)

	_, err = PCMToWAV([]byte{1, 2}, 16000, 1, 0)
	require.Error(t, err)
}
