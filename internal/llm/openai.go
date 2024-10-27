package llm

import (
	"bytes"
	"context"
	"discord-military-analyst-bot/internal/config"
	"encoding/json"
	"errors"
	"go.uber.org/zap"
	"io"
	"net/http"
	"time"
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

func NewOpenAIClient(endpoint string, token string) *OpenAIClient {
	provider := &OpenAIClient{
		Endpoint: endpoint,
		Token:    token,
	}

	return provider
}

func (provider *OpenAIClient) Infer(
	model string,
	system string,
	message string,
	history []HistoryItem,
	images []string,
	ctx context.Context,
) (error, string) {
	messages := make([]map[string]any, 0)
	systemRole := "system"
	if len(images) > 0 {
		systemRole = "user"
	}

	systemMessage := map[string]any{
		"role":    systemRole,
		"content": system,
	}

	messages = append(messages, systemMessage)
	for _, item := range history {
		if item.Content == "" {
			continue
		}

		role := "user"
		if item.IsBot {
			role = "assistant"
		}

		historyMessage := map[string]any{
			"role":    role,
			"content": item.Content,
		}

		messages = append(messages, historyMessage)
	}

	contentMessage := map[string]any{
		"role":    "user",
		"content": message,
	}

	if len(images) > 0 {
		var imageUrls []map[string]any
		for _, image := range images {
			imageUrls = append(imageUrls, map[string]any{
				"type": "image_url",
				"image_url": map[string]string{
					"url": image,
				},
			})
		}

		contentMessage = map[string]any{
			"role": "user",
			"content": append(
				[]map[string]any{
					{
						"type": "text",
						"text": message,
					},
				},
				imageUrls...,
			),
		}
	}

	messages = append(messages, contentMessage)
	requestBody := map[string]any{
		"messages": messages,
		"model":    model,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return err, ""
	}

	zap.L().Debug("openai request", zap.String("body", string(jsonBody)))
	req, err := http.NewRequestWithContext(ctx, "POST", config.Data.OpenAI.Endpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return err, ""
	}

	req.Header.Set("Authorization", "Bearer "+config.Data.OpenAI.ApiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 180 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		zap.L().Error("openai request failed", zap.Error(err))
		return err, ""
	}

	defer resp.Body.Close()
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
