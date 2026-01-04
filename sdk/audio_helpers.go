package vai

import "encoding/binary"

// PCMToWAV wraps raw PCM audio data with a WAV header.
//
// This is useful when you've collected PCM audio chunks from streaming TTS
// and want to save them to a file or use with players that require WAV format.
//
// Common TTS format: sampleRate=24000, bitsPerSample=16, channels=1
//
// Example:
//
//	// Collect PCM chunks from streaming
//	var pcmData []byte
//	for chunk := range audioChunks {
//	    pcmData = append(pcmData, chunk...)
//	}
//
//	// Convert to WAV for saving
//	wavData := vai.PCMToWAV(pcmData, 24000, 16, 1)
//	os.WriteFile("output.wav", wavData, 0644)
func PCMToWAV(pcmData []byte, sampleRate, bitsPerSample, channels int) []byte {
	dataLen := len(pcmData)
	byteRate := sampleRate * channels * bitsPerSample / 8
	blockAlign := channels * bitsPerSample / 8

	// WAV header is 44 bytes
	header := make([]byte, 44)

	// RIFF chunk descriptor
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], uint32(36+dataLen)) // File size - 8
	copy(header[8:12], "WAVE")

	// fmt sub-chunk
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)                       // Sub-chunk size (16 for PCM)
	binary.LittleEndian.PutUint16(header[20:22], 1)                        // Audio format (1 = PCM)
	binary.LittleEndian.PutUint16(header[22:24], uint16(channels))         // Number of channels
	binary.LittleEndian.PutUint32(header[24:28], uint32(sampleRate))       // Sample rate
	binary.LittleEndian.PutUint32(header[28:32], uint32(byteRate))         // Byte rate
	binary.LittleEndian.PutUint16(header[32:34], uint16(blockAlign))       // Block align
	binary.LittleEndian.PutUint16(header[34:36], uint16(bitsPerSample))    // Bits per sample

	// data sub-chunk
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], uint32(dataLen)) // Data size

	return append(header, pcmData...)
}

// Common audio format constants for TTS output
const (
	// DefaultTTSSampleRate is the common sample rate for TTS (24kHz)
	DefaultTTSSampleRate = 24000

	// DefaultTTSBitsPerSample is the common bit depth for TTS (16-bit)
	DefaultTTSBitsPerSample = 16

	// DefaultTTSChannels is the common channel count for TTS (mono)
	DefaultTTSChannels = 1
)

// PCMToWAVDefault wraps PCM data with a WAV header using default TTS format.
// Equivalent to PCMToWAV(pcmData, 24000, 16, 1)
func PCMToWAVDefault(pcmData []byte) []byte {
	return PCMToWAV(pcmData, DefaultTTSSampleRate, DefaultTTSBitsPerSample, DefaultTTSChannels)
}
