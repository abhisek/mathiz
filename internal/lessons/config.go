package lessons

// SessionCompressionThreshold is the character count threshold for
// triggering session-level error compression.
const SessionCompressionThreshold = 800

// Config holds lesson generation settings.
type Config struct {
	MaxTokens   int
	Temperature float64
}

// DefaultConfig returns sensible defaults for lesson generation.
func DefaultConfig() Config {
	return Config{
		MaxTokens:   512,
		Temperature: 0.5,
	}
}

// CompressorConfig holds compression settings.
type CompressorConfig struct {
	SessionMaxTokens int
	ProfileMaxTokens int
	Temperature      float64
}

// DefaultCompressorConfig returns sensible defaults for compression.
func DefaultCompressorConfig() CompressorConfig {
	return CompressorConfig{
		SessionMaxTokens: 256,
		ProfileMaxTokens: 512,
		Temperature:      0.3,
	}
}
