package llm

import (
	"context"
	"github.com/bwmarrin/discordgo"
)

type Client interface {
	Infer(
		model string,
		system string,
		message string,
		history []HistoryItem,
		images []string,
		ctx context.Context,
	) (error, string)
}

type HistoryItem struct {
	IsBot       bool
	Content     string
	Attachments []*discordgo.MessageAttachment
}
