package LLMProvider

import (
	"context"
)

type GroqClient struct {
	Token string
}

func CreateGroqClient(apiKey string) *GroqClient {
	provider := &GroqClient{
		Token: apiKey,
	}

	return provider
}

func (provider *GroqClient) Infer(model string, system string, message string, history []HistoryItem, ctx context.Context) (error, string) {
	return OpenAICompatibleInfer(
		model,
		message,
		history,
		system,
		provider.Token,
		"https://api.groq.com/openai/v1/chat/completions",
		ctx,
	)
}
