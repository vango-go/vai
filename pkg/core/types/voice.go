package types

// VoiceConfig configures the voice pipeline (STT → LLM → TTS).
type VoiceConfig struct {
	Input  *VoiceInputConfig  `json:"input,omitempty"`
	Output *VoiceOutputConfig `json:"output,omitempty"`
}

// VoiceInputConfig configures speech-to-text.
// Currently only Cartesia (ink-whisper) is supported.
type VoiceInputConfig struct {
	Model    string `json:"model,omitempty"`    // Model: "ink-whisper" (default)
	Language string `json:"language,omitempty"` // ISO language code (default: "en")
}

// VoiceOutputConfig configures text-to-speech.
// Currently only Cartesia (sonic-3) is supported.
type VoiceOutputConfig struct {
	Voice      string  `json:"voice"`                  // Cartesia voice ID (required)
	Speed      float64 `json:"speed,omitempty"`        // Speed: 0.6-1.5 (default: 1.0)
	Volume     float64 `json:"volume,omitempty"`       // Volume: 0.5-2.0 (default: 1.0)
	Emotion    string  `json:"emotion,omitempty"`      // Emotion (neutral, happy, sad, angry, etc.)
	Format     string  `json:"format,omitempty"`       // Output format: wav, mp3, pcm (default: wav)
	SampleRate int     `json:"sample_rate,omitempty"`  // Sample rate in Hz (default: 24000)
}

// Voice format constants
const (
	VoiceFormatMP3 = "mp3"
	VoiceFormatWAV = "wav"
	VoiceFormatPCM = "pcm"
)

// Supported emotions for Cartesia sonic-3
const (
	EmotionNeutral      = "neutral"
	EmotionHappy        = "happy"
	EmotionExcited      = "excited"
	EmotionEnthusiastic = "enthusiastic"
	EmotionCalm         = "calm"
	EmotionPeaceful     = "peaceful"
	EmotionAngry        = "angry"
	EmotionFrustrated   = "frustrated"
	EmotionSad          = "sad"
	EmotionMelancholic  = "melancholic"
	EmotionScared       = "scared"
	EmotionAnxious      = "anxious"
	EmotionConfident    = "confident"
	EmotionSerious      = "serious"
	EmotionCurious      = "curious"
)

// Supported languages for Cartesia STT (ink-whisper)
// This is a subset of the most common languages.
var SupportedSTTLanguages = []string{
	"en", "zh", "de", "es", "ru", "ko", "fr", "ja", "pt", "tr",
	"pl", "nl", "ar", "sv", "it", "id", "hi", "fi", "vi", "he",
	"uk", "el", "cs", "ro", "da", "hu", "th",
}

// Supported languages for Cartesia TTS (sonic-3)
var SupportedTTSLanguages = []string{
	"en", "fr", "de", "es", "pt", "zh", "ja", "hi", "it", "ko",
	"nl", "pl", "ru", "sv", "tr", "tl", "bg", "ro", "ar", "cs",
	"el", "fi", "hr", "ms", "sk", "da", "ta", "uk", "hu", "no",
	"vi", "bn", "th", "he", "ka", "id", "te", "gu", "kn", "ml",
	"mr", "pa",
}
