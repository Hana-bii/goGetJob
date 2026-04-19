package voiceinterview

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

func PCMToWAV(pcm []byte, sampleRate, channels, bitsPerSample int) ([]byte, error) {
	if sampleRate <= 0 {
		return nil, fmt.Errorf("sample rate must be positive")
	}
	if channels <= 0 {
		return nil, fmt.Errorf("channels must be positive")
	}
	if bitsPerSample <= 0 || bitsPerSample%8 != 0 {
		return nil, fmt.Errorf("bits per sample must be a positive multiple of 8")
	}
	byteRate := sampleRate * channels * bitsPerSample / 8
	blockAlign := channels * bitsPerSample / 8
	dataSize := len(pcm)

	buf := bytes.NewBuffer(make([]byte, 0, 44+dataSize))
	buf.WriteString("RIFF")
	_ = binary.Write(buf, binary.LittleEndian, uint32(36+dataSize))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(buf, binary.LittleEndian, uint16(channels))
	_ = binary.Write(buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(buf, binary.LittleEndian, uint32(byteRate))
	_ = binary.Write(buf, binary.LittleEndian, uint16(blockAlign))
	_ = binary.Write(buf, binary.LittleEndian, uint16(bitsPerSample))
	buf.WriteString("data")
	_ = binary.Write(buf, binary.LittleEndian, uint32(dataSize))
	_, _ = buf.Write(pcm)
	return buf.Bytes(), nil
}
