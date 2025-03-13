package llm

import (
	"bufio"
	"bytes"
	"context"
	"discord-military-analyst-bot/internal/config"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
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

type OpenAIStreamResponse struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

func NewOpenAIClient(endpoint string, token string) *OpenAIClient {
	provider := &OpenAIClient{
		Endpoint: endpoint,
		Token:    token,
	}

	return provider
}

func (c *OpenAIClient) InferStream(ctx context.Context, model string, system string, message string, history []HistoryItem) (<-chan StreamResponse, error) {
	responseChan := make(chan StreamResponse)

	messages := make([]map[string]any, 0)
	systemMessage := map[string]any{
		"role":    "system",
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

	messages = append(messages, contentMessage)
	requestBody := map[string]any{
		"messages":    messages,
		"model":       model,
		"stream":      true,
		"temperature": config.Data.OpenAI.Temperature,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	zap.L().Debug("openai stream request", zap.String("body", string(jsonBody)))
	req, err := http.NewRequestWithContext(ctx, "POST", config.Data.OpenAI.Endpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+config.Data.OpenAI.ApiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 180 * time.Second}

	go func() {
		defer close(responseChan)

		resp, err := client.Do(req)
		if err != nil {
			zap.L().Error("openai stream request failed", zap.Error(err))
			responseChan <- StreamResponse{Error: err}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			err := errors.New(string(body))
			responseChan <- StreamResponse{Error: err}
			return
		}

		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					break
				}
				responseChan <- StreamResponse{Error: err}
				return
			}

			line = strings.TrimSpace(line)
			if line == "" || line == "data: [DONE]" {
				continue
			}

			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			var streamResp OpenAIStreamResponse
			if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
				zap.L().Error("failed to unmarshal stream response", zap.Error(err), zap.String("data", data))
				continue
			}

			if len(streamResp.Choices) > 0 {
				content := streamResp.Choices[0].Delta.Content
				responseChan <- StreamResponse{Content: content, Done: false}
			}
		}

		responseChan <- StreamResponse{Done: true}
	}()

	return responseChan, nil
}

func (c *OpenAIClient) Infer(ctx context.Context, model string, system string, message string, history []HistoryItem) (string, error) {
	messages := make([]map[string]any, 0)
	systemMessage := map[string]any{
		"role":    "system",
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

	messages = append(messages, contentMessage)
	requestBody := map[string]any{
		"messages":    messages,
		"model":       model,
		"temperature": config.Data.OpenAI.Temperature,
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

// InferWithStream is a convenience method that collects all streaming chunks into a single response
func (c *OpenAIClient) InferWithStream(ctx context.Context, model string, system string, message string, history []HistoryItem, callback func(content string, done bool)) (string, error) {
	stream, err := c.InferStream(ctx, model, system, message, history)
	if err != nil {
		return "", err
	}

	var fullResponse strings.Builder

	for chunk := range stream {
		if chunk.Error != nil {
			return fullResponse.String(), chunk.Error
		}

		fullResponse.WriteString(chunk.Content)
		if callback != nil {
			callback(chunk.Content, chunk.Done)
		}
	}

	return fullResponse.String(), nil
}
