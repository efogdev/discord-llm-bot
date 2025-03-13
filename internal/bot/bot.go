package bot

import (
	"context"
	"discord-military-analyst-bot/internal/config"
	"discord-military-analyst-bot/internal/db"
	"discord-military-analyst-bot/internal/llm"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
)

var messageDB *db.MessageDB

type DiscordMessage struct {
	Session *discordgo.Session
	Message *discordgo.MessageCreate
}

// Close closes the database connection
func Close() {
	if messageDB != nil {
		err := messageDB.Close()
		if err != nil {
			zap.L().Error("failed to close database connection", zap.Error(err))
		}
	}
}

func Init() (*discordgo.Session, chan *DiscordMessage) {
	zap.L().Debug("initializing bot")

	// Initialize database
	var err error
	messageDB, err = db.New(config.Data.Database.Path)
	if err != nil {
		zap.L().Panic("failed to initialize database", zap.Error(err))
		return nil, nil
	}

	discord, err := discordgo.New("Bot " + config.Data.Discord.Token)
	queue := make(chan *DiscordMessage, 128)

	if err != nil {
		zap.L().Panic("incorrect Discord token", zap.Error(err))
		return nil, nil
	}

	discord.AddHandler(func(session *discordgo.Session, message *discordgo.MessageCreate) {
		if message.Author.ID == config.Data.Discord.BotId {
			return
		}

		if message.GuildID == "" && !config.Data.Discord.AllowDM {
			return
		}

		queue <- &DiscordMessage{session, message}
	})

	discord.AddHandler(func(session *discordgo.Session, reaction *discordgo.MessageReactionAdd) {
		if reaction.Emoji.Name == config.Data.Discord.BonkEmojiName && (reaction.Member.User.ID == config.Data.Discord.SuperuserId || config.Data.Discord.BonkFromAnyone) {
			err = session.ChannelMessageDelete(reaction.MessageReaction.ChannelID, reaction.MessageReaction.MessageID)
			zap.L().Info("got bonk, removing message", zap.Any("reaction", reaction))

			if err != nil {
				zap.L().Error("error deleting message", zap.Error(err))
			}
		}
	})

	discord.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsGuildMessageReactions | discordgo.IntentDirectMessages

	err = discord.Open()
	if err != nil {
		zap.L().Panic("error initializing Discord bot", zap.Error(err))
		return nil, nil
	}

	return discord, queue
}

func FindURL(content string) string {
	re := regexp.MustCompile(`https://[^\s]+`)
	matches := re.FindAllString(content, -1)

	if len(matches) > 0 {
		return matches[0]
	}

	return ""
}

func FetchHistory(message *discordgo.MessageCreate, session *discordgo.Session, botId string) (error, []llm.HistoryItem) {
	botMentioned := false

	for _, mention := range message.Mentions {
		if mention.ID == botId {
			botMentioned = true
		}
	}

	if message.ReferencedMessage != nil && message.ReferencedMessage.Author.ID == botId {
		botMentioned = true
	}

	if !botMentioned {
		return errors.New("bot not mentioned"), nil
	}

	// First try to get history from the database
	if messageDB != nil && message.ID != "" {
		// Save the current message to the database
		if message.ReferencedMessage != nil {
			// Save the referenced message first if it exists
			err := messageDB.SaveMessage(message.ReferencedMessage, message.ReferencedMessage.Author.ID == botId)
			if err != nil {
				zap.L().Error("failed to save referenced message to database", zap.Error(err))
			}
		}

		// Try to get history from the database
		history, err := messageDB.GetMessageHistory(message.ID, botId)
		if err == nil && len(history) > 0 {
			zap.L().Debug("retrieved message history from database", zap.Int("count", len(history)))
			return nil, history
		}
	}

	// If not found in database or database is not initialized, fetch from Discord
	zap.L().Debug("fetching message history from Discord")
	var history []llm.HistoryItem

	current := message.ReferencedMessage
	for current != nil {
		// Save message to database as we fetch it
		if messageDB != nil {
			err := messageDB.SaveMessage(current, current.Author.ID == botId)
			if err != nil {
				zap.L().Error("failed to save message to database", zap.Error(err))
			}
		}

		history = append([]llm.HistoryItem{{
			IsBotMessage: current.Author.ID == botId,
			Content:      current.Content,
			Attachments:  current.Attachments,
		}}, history...)

		if current.Type == discordgo.MessageTypeReply {
			current, _ = session.ChannelMessage(current.ChannelID, current.ID)
			current = current.ReferencedMessage
		} else {
			current = nil
		}
	}

	return nil, history
}

func ReadSystemPrompt() (error, string) {
	systemPrompt, err := os.ReadFile(filepath.Join("internal", "bot", "system-prompt.txt"))
	if err != nil {
		return err, ""
	}

	return nil, strings.TrimSpace(string(systemPrompt))
}

func HandleMessage(msg *discordgo.MessageCreate, session *discordgo.Session, client llm.Client, ctx context.Context) {
	// Save the incoming message to the database
	if messageDB != nil {
		err := messageDB.SaveMessage(msg.Message, false)
		if err != nil {
			zap.L().Error("failed to save message to database", zap.Error(err))
		}
	}

	err, history := FetchHistory(msg, session, config.Data.Discord.BotId)
	if err != nil && msg.GuildID != "" {
		return
	}

	zap.L().Debug("message received", zap.String("text", msg.Content))

	ignoreSystemPrompt := false
	if config.Data.Discord.IgnoreSystemKeyword != "" {
		if strings.Contains(msg.Content, config.Data.Discord.IgnoreSystemKeyword) {
			ignoreSystemPrompt = true
		}

		for _, item := range history {
			if !item.IsBotMessage && strings.Contains(item.Content, config.Data.Discord.IgnoreSystemKeyword) {
				ignoreSystemPrompt = true
			}
		}
	}

	if msg.GuildID == "" && config.Data.Discord.DisableSystemForDM {
		ignoreSystemPrompt = true
	}

	url := FindURL(msg.Content)
	if url == "" && msg.ReferencedMessage != nil {
		url = FindURL(msg.ReferencedMessage.Content)
	}

	system := "Keep your response short. Be concise and say only important things, not meaningful words (water)."
	err = nil
	if !ignoreSystemPrompt {
		err, system = ReadSystemPrompt()
	} else {
		zap.L().Info("ignoring system prompt")
	}

	if err != nil {
		zap.L().Panic("error reading prompt file", zap.Error(err))
	}

	if config.Data.Discord.Typing {
		_ = session.ChannelTyping(msg.ChannelID)
	}

	llmRequest := ""
	msgContent := msg.Content
	if ignoreSystemPrompt {
		msgContent = strings.ReplaceAll(msg.Content, config.Data.Discord.IgnoreSystemKeyword, "")
	}

	if len(history) <= 1 && url != "" {
		zap.L().Info("found url to parse", zap.String("url", url))

		if ignoreSystemPrompt {
			history = append(history, llm.HistoryItem{
				Content:      strings.ReplaceAll(msgContent, url, ""),
				IsBotMessage: false,
			})
		}

		_ = session.MessageReactionAdd(msg.ChannelID, msg.ID, "👀")
		err, parsedContent := ParseURL(url)
		if err != nil {
			_, _ = session.ChannelMessageSendReply(msg.ChannelID, "Your link is bullshit bro.", msg.MessageReference)
			return
		}

		zap.L().Debug("content parser success")
		content := fmt.Sprintf("URL: %s\nContent:\n%s", url, parsedContent)
		llmRequest = content
	} else {
		zap.L().Info("no url found", zap.String("message", msgContent))
		llmRequest = msgContent
	}

	// Get message history
	var allHistory []llm.HistoryItem
	if ignoreSystemPrompt {
		// When ignoring system prompt, only use the immediate parent message
		if len(history) > 0 {
			allHistory = history[:1]
		} else {
			allHistory = history
		}
	} else {
		// Get full history for normal mode
		if messageDB != nil {
			allHistory, err = messageDB.GetAllRelatedMessages(msg.ID, config.Data.Discord.BotId)
			if err != nil {
				zap.L().Error("failed to get all related messages", zap.Error(err))
				allHistory = history // Fallback to direct history
			}
		} else {
			allHistory = history
		}
	}

	zap.L().Debug("inferencing with streaming", zap.String("content", llmRequest), zap.Any("history", allHistory))

	var sentMessage *discordgo.Message
	var messageCreated bool

	// Use streaming API
	openaiClient, ok := client.(*llm.OpenAIClient)
	if !ok {
		zap.L().Error("client is not an OpenAIClient, falling back to non-streaming")
		llmResponse, inferErr := client.Infer(ctx, config.Data.Model, system, llmRequest, allHistory)
		if inferErr != nil {
			zap.L().Error("error while trying to infer an llm", zap.Error(inferErr))
			return
		}

		if len(llmResponse) > 1999 {
			llmResponse = llmResponse[:1999]
		}

		if llmResponse == "" {
			zap.L().Warn("empty llm response")
			return
		}

		// Update the message with the full response
		_, err = session.ChannelMessageEdit(sentMessage.ChannelID, sentMessage.ID, llmResponse)
		if err != nil {
			zap.L().Error("error updating message", zap.Error(err))
		}

		return
	}

	// Use streaming for OpenAI client
	var fullResponse strings.Builder
	var lastUpdateTime time.Time
	updateInterval := 500 * time.Millisecond // Update message every 500ms

	_, streamErr := openaiClient.InferWithStream(ctx, config.Data.Model, system, llmRequest, allHistory,
		func(content string, done bool) {
			fullResponse.WriteString(content)
			currentTime := time.Now()

			// Create initial message when we receive the first content
			if !messageCreated && content != "" {
				var initialErr error
				if msg.GuildID == "" {
					sentMessage, initialErr = session.ChannelMessageSend(msg.ChannelID, content)
				} else {
					sentMessage, initialErr = session.ChannelMessageSendReply(msg.ChannelID, content, msg.Reference())
				}

				if initialErr != nil {
					zap.L().Error("error sending initial message", zap.Error(initialErr))
					return
				}

				// Save the initial bot response to the database
				if messageDB != nil && sentMessage != nil {
					err := messageDB.SaveMessage(sentMessage, true)
					if err != nil {
						zap.L().Error("failed to save initial bot response to database", zap.Error(err))
					}
				}

				messageCreated = true
				lastUpdateTime = currentTime
				return
			}

			// Update the message if enough time has passed or if it's the final update
			if messageCreated && (done || currentTime.Sub(lastUpdateTime) >= updateInterval) {
				responseText := fullResponse.String()

				// Truncate if needed
				if len(responseText) > 1999 {
					responseText = responseText[:1999]
				}

				// Only update if there's content
				if responseText != "" {
					_, err := session.ChannelMessageEdit(sentMessage.ChannelID, sentMessage.ID, responseText)
					if err != nil {
						zap.L().Error("error updating message", zap.Error(err))
					}
					lastUpdateTime = currentTime
				}
			}
		})

	if streamErr != nil {
		zap.L().Error("error while streaming from llm", zap.Error(streamErr))
		// Try to update the message with the error
		_, _ = session.ChannelMessageEdit(sentMessage.ChannelID, sentMessage.ID, "Error generating response")
		return
	}

	// Final update to the message
	finalResponse := fullResponse.String()
	if len(finalResponse) > 1999 {
		finalResponse = finalResponse[:1999]
	}

	if finalResponse == "" {
		zap.L().Warn("empty llm response")
		_, _ = session.ChannelMessageEdit(sentMessage.ChannelID, sentMessage.ID, "No response generated")
		return
	}

	// Update the message with the final response
	updatedMessage, editErr := session.ChannelMessageEdit(sentMessage.ChannelID, sentMessage.ID, finalResponse)
	if editErr != nil {
		zap.L().Error("error updating final message", zap.Error(editErr))
		return
	}

	// Update the saved message in the database
	if messageDB != nil && updatedMessage != nil {
		err := messageDB.SaveMessage(updatedMessage, true)
		if err != nil {
			zap.L().Error("failed to save updated bot response to database", zap.Error(err))
		}
	}
}

func ParseURL(url string) (error, string) {
	cmd := exec.Command("node", "index.js", url)
	cmd.Dir = filepath.Join(".", "content-from-webpage")

	output, err := cmd.Output()
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			if exitError.ExitCode() != 0 {
				zap.L().Warn("content parser failed", zap.Int("errorCode", exitError.ExitCode()))
			}
		}

		return err, ""
	}

	return nil, strings.TrimSpace(string(output))
}
