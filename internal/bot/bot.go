package bot

import (
	"context"
	"discord-military-analyst-bot/internal/config"
	"discord-military-analyst-bot/internal/db"
	"discord-military-analyst-bot/internal/llm"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

var imageContentTypes = []string{
	"image/jpeg",
	"image/png",
	"image/gif",
	"image/webp",
	"image/svg+xml",
	"image/tiff",
	"image/bmp",
	"image/x-icon",
	"image/vnd.microsoft.icon",
	"image/heic",
	"image/heif",
	"image/avif",
	"image/jxl",
}

const (
	ImgDefaultWidth  = 384
	ImgDefaultHeight = 288
	ImgDefaultCount  = 1
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

	// image generation mode
	if config.Data.Discord.MakeImageKeyword != "" && strings.Contains(msg.Content, config.Data.Discord.MakeImageKeyword) {
		_ = session.MessageReactionAdd(msg.ChannelID, msg.ID, "👨🏻‍🎨")

		system := strings.ReplaceAll(msg.Content, config.Data.Discord.MakeImageKeyword, "")
		images, err := client.MakeImage(ctx, config.Data.ImageModel, system, ImgDefaultWidth, ImgDefaultHeight, ImgDefaultCount)
		if err != nil {
			zap.L().Error("error making image", zap.Error(err))
			return
		}

		if len(images) == 0 {
			zap.L().Error("no images in response")
			return
		}

		imageUrl := images[0]
		resp, err := http.Get(imageUrl)
		if err != nil {
			zap.L().Error("failed to download image", zap.Error(err))
			return
		}
		defer resp.Body.Close()

		ext := filepath.Ext(imageUrl)
		if ext == "" {
			ext = ".png"
		}
		filename := uuid.Must(uuid.NewV7()).String() + ext

		_, err = session.ChannelMessageSendComplex(msg.ChannelID, &discordgo.MessageSend{
			Files: []*discordgo.File{{
				Name:        filename,
				ContentType: resp.Header.Get("Content-Type"),
				Reader:      resp.Body,
			}},
			Reference: msg.Reference(),
		})
		if err != nil {
			zap.L().Error("failed to send message", zap.Error(err))
			return
		}

		return
	}

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
	llmResponse := ""
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

	// images := FindImages(msg, history)
	var images []string
	if len(images) > 0 {
		zap.L().Info("attaching images", zap.Any("images", images))
	}

	// Get all related messages from the database for context
	var allHistory []llm.HistoryItem
	if messageDB != nil {
		allHistory, err = messageDB.GetAllRelatedMessages(msg.ID, config.Data.Discord.BotId)
		if err != nil {
			zap.L().Error("failed to get all related messages", zap.Error(err))
			allHistory = history // Fallback to direct history
		}
	} else {
		allHistory = history
	}

	zap.L().Debug("inferencing", zap.String("content", llmRequest), zap.Any("history", allHistory))
	llmResponse, err = client.Infer(ctx, config.Data.Model, system, llmRequest, allHistory, images)

	if err != nil {
		zap.L().Error("error while trying to infer an llm", zap.Error(err))
		return
	}

	if len(llmResponse) > 1999 {
		llmResponse = llmResponse[:1999]
	}

	if llmResponse == "" {
		zap.L().Warn("empty llm response")
		return
	}

	zap.L().Info("sending reply", zap.String("text", llmResponse))

	var sentMessage *discordgo.Message
	if msg.GuildID == "" {
		sentMessage, err = session.ChannelMessageSend(msg.ChannelID, llmResponse)
	} else {
		sentMessage, err = session.ChannelMessageSendReply(msg.ChannelID, llmResponse, msg.Reference())
	}

	// Save the bot's response to the database
	if messageDB != nil && sentMessage != nil {
		err := messageDB.SaveMessage(sentMessage, true)
		if err != nil {
			zap.L().Error("failed to save bot response to database", zap.Error(err))
		}
	}

	if err != nil {
		zap.L().Error("error sending reply", zap.String("text", err.Error()))
		return
	}
}

// FindImages actually finds only 1 image for now
func FindImages(msg *discordgo.MessageCreate, history []llm.HistoryItem) []string {
	for _, attach := range msg.Attachments {
		if !slices.Contains(imageContentTypes, attach.ContentType) {
			continue
		}

		return []string{attach.URL}
	}

	for _, historyItem := range history {
		for _, attach := range historyItem.Attachments {
			if !slices.Contains(imageContentTypes, attach.ContentType) {
				continue
			}

			return []string{attach.URL}
		}
	}

	return []string{}
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
