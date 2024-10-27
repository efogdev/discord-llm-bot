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

const ImgDefaultSteps = 10

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

type OpenAIImageResponse struct {
	Data []struct {
		Url string `json:"url"`
	} `json:"data"`
}

func NewOpenAIClient(endpoint string, token string) *OpenAIClient {
	provider := &OpenAIClient{
		Endpoint: endpoint,
		Token:    token,
	}

	return provider
}

func (c *OpenAIClient) MakeImage(ctx context.Context, model string, system string, width uint, height uint, count uint8) ([]string, error) {
	if count == 0 || width == 0 || height == 0 {
		return nil, errors.New("dimensions or count incorrect")
	}

	requestBody := map[string]any{
		"steps":  ImgDefaultSteps,
		"n":      count,
		"model":  model,
		"prompt": system,
		"width":  width,
		"height": height,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	zap.L().Debug("image request", zap.String("body", string(jsonBody)))
	req, err := http.NewRequestWithContext(ctx, "POST", config.Data.OpenAI.ImageEndpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+config.Data.OpenAI.ApiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 180 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		zap.L().Error("image request failed", zap.Error(err))
		return nil, err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(string(body))
	}

	zap.L().Debug("image response", zap.String("body", string(body)))
	var result OpenAIImageResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	var images []string
	for idx := range result.Data {
		images = append(images, result.Data[idx].Url)
	}

	return images, nil
}

func (c *OpenAIClient) Infer(ctx context.Context, model string, system string, message string, history []HistoryItem, images []string) (string, error) {
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
		if item.IsBotMessage {
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
		return "", err
	}

	zap.L().Debug("openai request", zap.String("body", string(jsonBody)))
	req, err := http.NewRequestWithContext(ctx, "POST", config.Data.OpenAI.Endpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+config.Data.OpenAI.ApiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 180 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		zap.L().Error("openai request failed", zap.Error(err))
		return "", err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", errors.New(string(body))
	}

	var result OpenAIResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	return result.Choices[0].Message.Content, nil
}
