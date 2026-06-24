// Package constant holds compile-time enumerations shared across the platform.
package constant

// Channel types — values are kept identical to new-api / AGT so the numbers we
// store and forward are wire-compatible with the downstream platform.
// (Confirmed against new-api/constant/channel.go.)
const (
	ChannelTypeUnknown   = 0
	ChannelTypeOpenAI    = 1  // OpenAI
	ChannelTypeAzure     = 3  // Azure OpenAI
	ChannelTypeAnthropic = 14 // Anthropic Claude
	ChannelTypeGemini    = 24 // Google Gemini
	ChannelTypeAws       = 33 // AWS Bedrock Claude
	ChannelTypeVertexAi  = 41 // Google Vertex AI
)

// SupportedChannelTypes is the closed set of providers this platform accepts.
// Anything outside this set is rejected at the API boundary.
var SupportedChannelTypes = map[int]string{
	ChannelTypeOpenAI:    "OpenAI",
	ChannelTypeAzure:     "Azure OpenAI",
	ChannelTypeAnthropic: "Anthropic Claude",
	ChannelTypeGemini:    "Google Gemini",
	ChannelTypeAws:       "AWS Claude (Bedrock)",
	ChannelTypeVertexAi:  "Google Vertex AI",
}

// IsSupportedChannelType reports whether t is one of the six accepted providers.
func IsSupportedChannelType(t int) bool {
	_, ok := SupportedChannelTypes[t]
	return ok
}
