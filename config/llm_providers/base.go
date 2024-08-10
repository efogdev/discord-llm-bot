package llm_providers

type LLMProviderClient interface {
	SetSystem(message string)
	Infer(model string, message string) string
}

type NoopLLMProviderClient struct{}

func CreateNoopLLMProviderClient() NoopLLMProviderClient {
	return NoopLLMProviderClient{}
}

func (provider NoopLLMProviderClient) SetSystem(message string)                  {}
func (provider NoopLLMProviderClient) Infer(model string, message string) string { return "" }
