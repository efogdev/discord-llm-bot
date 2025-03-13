package llm

import (
	"context"

	"github.com/bwmarrin/discordgo"
)

// StreamResponse represents a chunk of the streaming response
type StreamResponse struct {
	Content string
	Done    bool
	Error   error
}

// StreamClient is an optional interface that clients can implement to support streaming
type StreamClient interface {
	// InferStream returns a channel that streams response chunks
	InferStream(ctx context.Context, model string, system string, message string, history []HistoryItem) (<-chan StreamResponse, error)

	// InferWithStream is a convenience method that collects all streaming chunks and calls the callback for each chunk
	InferWithStream(ctx context.Context, model string, system string, message string, history []HistoryItem, callback func(content string, done bool)) (string, error)
}

type Client interface {
	Infer(ctx context.Context, model string, system string, message string, history []HistoryItem) (string, error)
}

type HistoryItem struct {
	Content      string
	IsBotMessage bool
	Attachments  []*discordgo.MessageAttachment
}
