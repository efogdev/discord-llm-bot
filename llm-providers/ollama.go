package LLMProvider

import (
	"context"
)

type OllamaClient struct {
	Endpoint string
	Token    string
}

func CreateOllamaProvider(endpoint string, token string) *OllamaClient {
	provider := &OllamaClient{
		Endpoint: endpoint,
		Token:    token,
	}

	return provider
}

func (provider *OllamaClient) Infer(model string, system string, message string, history []HistoryItem, ctx context.Context) (error, string) {
	return OpenAICompatibleInfer(
		model,
		message,
		history,
		system,
		provider.Token,
		provider.Endpoint,
		ctx,
	)
}
