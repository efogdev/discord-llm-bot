package LLMProvider

import (
	"context"
)

type OllamaClient struct {
	Endpoint string
}

func CreateOllamaProvider(endpoint string) *OllamaClient {
	provider := &OllamaClient{
		Endpoint: endpoint,
	}

	return provider
}

func (provider *OllamaClient) Infer(model string, system string, message string, history []HistoryItem, ctx context.Context) (error, string) {
	return OpenAICompatibleInfer(
		model,
		message,
		history,
		system,
		"",
		provider.Endpoint,
		ctx,
	)
}
