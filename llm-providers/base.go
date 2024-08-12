package LLMProvider

import (
	"context"
)

type Client interface {
	Infer(model string, system string, message string, history []HistoryItem, ctx context.Context) (error, string)
}

type HistoryItem struct {
	IsBot   bool
	Content string
}

type NoopClient struct{}

func CreateNoopClient() *NoopClient {
	return &NoopClient{}
}
func (provider *NoopClient) Infer(model string, system string, message string, history []HistoryItem, ctx context.Context) (error, string) {
	return nil, ""
}
