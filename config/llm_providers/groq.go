package llm_providers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

type GroqProvider struct {
	token  string
	system string
}

func CreateGroqProvider(apiKey string) GroqProvider {
	provider := GroqProvider{
		token: apiKey,
	}

	return provider
}

func (provider GroqProvider) SetSystem(message string) {
	provider.system = message
}

func (provider GroqProvider) Infer(model string, message string) string {
	messages := make([]map[string]string, 0)

	systemMessage := map[string]string{
		"role":    "system",
		"content": provider.system,
	}

	contentMessage := map[string]string{
		"role":    "user",
		"content": message,
	}

	messages = append(messages, systemMessage, contentMessage)
	requestBody := map[string]interface{}{
		"messages": messages,
		"model":    model,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return ""
	}

	req, err := http.NewRequest("POST", "https://api.groq.com/openai/v1/chat/completions", bytes.NewBuffer(jsonBody))
	if err != nil {
		return ""
	}

	req.Header.Set("Authorization", "Bearer "+provider.token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		return ""
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return ""
	}

	if choices, ok := result["choices"].([]interface{}); ok && len(choices) > 0 {
		if firstChoice, ok := choices[0].(map[string]interface{}); ok {
			if content, ok := firstChoice["message"].(map[string]interface{})["content"].(string); ok {
				return content
			}
		}
	}

	return ""
}
