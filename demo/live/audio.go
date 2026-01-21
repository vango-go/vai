package main

import (
	"log"
	"sync"

	"github.com/ebitengine/oto/v3"
	"github.com/gen2brain/malgo"
)

const (
	sampleRate = 24000
	channels   = 1
)

// initAudio sets up microphone input and speaker output.
// Returns a mic reader, speaker writer, and cleanup function.
func initAudio() (*micReader, *speakerWriter, func()) {
	// Initialize malgo for microphone
	malgoConfig := malgo.ContextConfig{}
	malgoConfig.ThreadPriority = malgo.ThreadPriorityRealtime

	malgoCtx, err := malgo.InitContext(nil, malgoConfig, nil)
	if err != nil {
		log.Fatalf("Failed to init audio context: %v", err)
	}

	// Create microphone reader
	mic := newMicReader(malgoCtx.Context, sampleRate, channels)

	// Initialize oto for speaker with low-latency buffer
	// At 24kHz mono 16-bit: 4800 bytes = 100ms of audio
	// Smaller buffer = lower latency but risk of glitches
	otoOpts := &oto.NewContextOptions{
		SampleRate:   sampleRate,
		ChannelCount: channels,
		Format:       oto.FormatSignedInt16LE,
		BufferSize:   4800, // ~100ms buffer for low latency
	}
	otoCtx, ready, err := oto.NewContext(otoOpts)
	if err != nil {
		log.Fatalf("Failed to init speaker: %v", err)
	}
	<-ready

	// Create speaker writer
	speaker := newSpeakerWriter(otoCtx)

	cleanup := func() {
		mic.Close()
		speaker.Close()
		malgoCtx.Uninit()
	}

	return mic, speaker, cleanup
}

// micReader captures audio from the microphone.
type micReader struct {
	device *malgo.Device
	buf    []byte
	mu     sync.Mutex
	cond   *sync.Cond
}

func newMicReader(ctx malgo.Context, sampleRate, channels int) *micReader {
	m := &micReader{
		buf: make([]byte, 0, sampleRate*2), // 1 second buffer
	}
	m.cond = sync.NewCond(&m.mu)

	deviceConfig := malgo.DefaultDeviceConfig(malgo.Capture)
	deviceConfig.Capture.Format = malgo.FormatS16
	deviceConfig.Capture.Channels = uint32(channels)
	deviceConfig.SampleRate = uint32(sampleRate)
	deviceConfig.PeriodSizeInMilliseconds = 20

	callbacks := malgo.DeviceCallbacks{
		Data: func(_, pInputSamples []byte, _ uint32) {
			m.mu.Lock()
			m.buf = append(m.buf, pInputSamples...)
			m.mu.Unlock()
			m.cond.Signal()
		},
	}

	device, err := malgo.InitDevice(ctx, deviceConfig, callbacks)
	if err != nil {
		log.Fatalf("Failed to init microphone: %v", err)
	}
	m.device = device

	if err := device.Start(); err != nil {
		log.Fatalf("Failed to start microphone: %v", err)
	}

	return m
}

func (m *micReader) Read(p []byte) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	for len(m.buf) == 0 {
		m.cond.Wait()
	}

	n := copy(p, m.buf)
	m.buf = m.buf[n:]
	return n
}

func (m *micReader) Close() {
	if m.device != nil {
		m.device.Stop()
		m.device.Uninit()
	}
}

// speakerWriter plays audio through the speaker.
type speakerWriter struct {
	otoCtx  *oto.Context
	player  *oto.Player
	buf     []byte
	mu      sync.Mutex
	cond    *sync.Cond
	playing bool
	closed  bool
}

func newSpeakerWriter(ctx *oto.Context) *speakerWriter {
	s := &speakerWriter{
		otoCtx: ctx,
		buf:    make([]byte, 0, sampleRate*4), // 2 second buffer capacity
	}
	s.cond = sync.NewCond(&s.mu)
	// Don't create/start player yet - wait until we have audio data
	return s
}

func (s *speakerWriter) Write(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.buf = append(s.buf, data...)

	// Start playing on first write.
	// Pre-buffering is handled by SDK's AudioOutput, so we can start immediately.
	if !s.playing && !s.closed {
		s.playing = true
		s.player = s.otoCtx.NewPlayer(s)
		s.player.Play()
	}

	// Signal that data is available
	s.cond.Signal()
}

// Read implements io.Reader for oto.Player.
// This is called by oto to pull audio data for playback.
func (s *speakerWriter) Read(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Wait for data or close
	for len(s.buf) == 0 && !s.closed {
		s.cond.Wait()
	}

	if s.closed && len(s.buf) == 0 {
		// Return silence on close to let oto drain gracefully
		for i := range p {
			p[i] = 0
		}
		return len(p), nil
	}

	n := copy(p, s.buf)
	s.buf = s.buf[n:]
	return n, nil
}

func (s *speakerWriter) Close() {
	s.mu.Lock()
	s.closed = true
	s.cond.Broadcast() // Wake up any waiting Read
	s.mu.Unlock()

	if s.player != nil {
		s.player.Close()
	}
}

// Flush discards all pending audio in the buffer and stops playback.
// This is called when the user continues speaking during grace period
// or when a real interrupt is confirmed, to immediately stop playback.
func (s *speakerWriter) Flush() {
	s.mu.Lock()
	s.buf = s.buf[:0] // Clear our buffer

	// Stop current playback so next audio starts fresh
	if s.player != nil && s.playing {
		s.playing = false
		player := s.player
		s.player = nil
		s.mu.Unlock()

		// Pause immediately to stop audio, then reset to clear oto's internal buffer.
		// This ensures old audio doesn't overlap with new audio after grace period.
		player.Pause()
		player.Reset()
		player.Close()
		return
	}
	s.mu.Unlock()
}
