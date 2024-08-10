package LLMProvider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"go.uber.org/zap"
	"io"
	"net/http"
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

func OpenAICompatibleInfer(
	model string,
	message string,
	history []HistoryItem,
	system string,
	token string,
	apiUrl string,
	ctx context.Context,
) (error, string) {
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

	req, err := http.NewRequestWithContext(ctx, "POST", apiUrl, bytes.NewBuffer(jsonBody))
	if err != nil {
		return err, ""
	}

	req.Header.Set("Authorization", "Bearer "+token)
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

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return err, ""
	}

	if choices, ok := result["choices"].([]interface{}); ok && len(choices) > 0 {
		if firstChoice, ok := choices[0].(map[string]interface{}); ok {
			if content, ok := firstChoice["message"].(map[string]interface{})["content"].(string); ok {
				return nil, content
			}
		}
	}

	return errors.New("received invalid API response"), ""
}
