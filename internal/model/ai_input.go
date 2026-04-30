package model

// AIImageInput is the public mirror of the package-private aiImageInput in
// internal/service. It exists so the ai_conversation package can pass image
// data to AIService.CallProviderStreamGeneric without importing internal types.
type AIImageInput struct {
	MIMEType string
	Data     string // base64-encoded
}
