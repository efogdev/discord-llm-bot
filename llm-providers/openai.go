package LLMProvider

import (
	"bytes"
	"context"
	"discord-military-analyst-bot/config"
	"encoding/json"
	"errors"
	"go.uber.org/zap"
	"io"
	"net/http"
)

type OpenAIClient struct {
	Endpoint string
	Token    string
}

type OpenAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func CreateOpenAIProvider(endpoint string, token string) *OpenAIClient {
	provider := &OpenAIClient{
		Endpoint: endpoint,
		Token:    token,
	}

	return provider
}

func (provider *OpenAIClient) Infer(model string, system string, message string, history []HistoryItem, ctx context.Context) (error, string) {
	messages := make([]map[string]string, 0)

	systemMessage := map[string]string{
		"role":    "system",
		"content": system,
	}

	messages = append(messages, systemMessage)

	for _, item := range history {
		role := "user"
		if item.IsBot {
			role = "assistant"
		}

		historyMessage := map[string]string{
			"role":    role,
			"content": item.Content,
		}

		messages = append(messages, historyMessage)
	}

	contentMessage := map[string]string{
		"role":    "user",
		"content": message,
	}

	messages = append(messages, contentMessage)

	requestBody := map[string]interface{}{
		"messages": messages,
		"model":    model,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return err, ""
	}

	req, err := http.NewRequestWithContext(ctx, "POST", config.Data.OpenAI.Endpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return err, ""
	}

	req.Header.Set("Authorization", "Bearer "+config.Data.OpenAI.ApiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)

	if err != nil {
		return err, ""
	}

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			zap.L().Debug("unexpeceted error while closing response body", zap.Error(err))
		}
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err, ""
	}
	if resp.StatusCode != http.StatusOK {
		return errors.New(string(body)), ""
	}

	var result OpenAIResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return err, ""
	}
	
	return nil, result.Choices[0].Message.Content
}
