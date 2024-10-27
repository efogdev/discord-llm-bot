package llm

import (
	"context"
	"github.com/bwmarrin/discordgo"
)

type Client interface {
	Infer(ctx context.Context, model string, system string, message string, history []HistoryItem, images []string) (string, error)
	MakeImage(ctx context.Context, model string, system string, width uint, height uint, count uint8) ([]string, error)
}

type HistoryItem struct {
	Content      string
	IsBotMessage bool
	Attachments  []*discordgo.MessageAttachment
}
