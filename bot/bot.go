package bot

import (
	"context"
	"discord-military-analyst-bot/config"
	"discord-military-analyst-bot/llm-providers"
	"errors"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type DiscordMessage struct {
	Session *discordgo.Session
	Message *discordgo.MessageCreate
}

func Init() (*discordgo.Session, chan *DiscordMessage) {
	zap.L().Debug("initializing bot")

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

func FetchHistory(message *discordgo.MessageCreate, session *discordgo.Session, botId string) (error, []LLMProvider.HistoryItem) {
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

	var history []LLMProvider.HistoryItem

	current := message.ReferencedMessage
	for current != nil {
		cleanContent := regexp.MustCompile(`<@([0-9]+?)>`).ReplaceAllString(current.Content, "")
		history = append([]LLMProvider.HistoryItem{{
			IsBot:   current.Author.ID == botId,
			Content: cleanContent,
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
	systemPrompt, err := os.ReadFile(filepath.Join("bot", "system-prompt.txt"))
	if err != nil {
		return err, ""
	}

	return nil, strings.TrimSpace(string(systemPrompt))
}

func HandleMessage(
	msg *discordgo.MessageCreate,
	session *discordgo.Session,
	client LLMProvider.Client,
	ctx context.Context,
) {
	url := ""

	err, history := FetchHistory(msg, session, config.Data.Discord.BotId)
	if err != nil && msg.GuildID != "" {
		zap.L().Debug("no bot mention, ignoring")
		return
	}

	zap.L().Debug("message received", zap.String("text", msg.Content))

	ignoreSystemPrompt := false

	if config.Data.Discord.KeywordToIgnoreSystem != "" {
		if strings.Contains(msg.Content, config.Data.Discord.KeywordToIgnoreSystem) {
			ignoreSystemPrompt = true
		}

		for _, item := range history {
			if !item.IsBot && strings.Contains(item.Content, config.Data.Discord.KeywordToIgnoreSystem) {
				ignoreSystemPrompt = true
			}
		}
	}

	if msg.GuildID == "" && config.Data.Discord.DisableSystemForDM {
		ignoreSystemPrompt = true
	}

	url = FindURL(msg.Content)

	if url == "" && msg.ReferencedMessage != nil {
		url = FindURL(msg.ReferencedMessage.Content)
	}

	system := ""
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
	if len(history) <= 1 && url != "" {
		zap.L().Info("found url to parse", zap.String("url", url))

		if ignoreSystemPrompt {
			sanitizedContent := strings.ReplaceAll(msg.Content, config.Data.Discord.KeywordToIgnoreSystem, "")
			sanitizedContent = strings.ReplaceAll(sanitizedContent, url, "")

			history = append(history, LLMProvider.HistoryItem{
				Content: sanitizedContent,
				IsBot:   false,
			})
		}

		_ = session.MessageReactionAdd(msg.ChannelID, msg.ID, "👀")

		err, parsedContent := ParseURL(url)
		if err != nil {
			_, _ = session.ChannelMessageSendReply(msg.ChannelID, "Your link is bullshit bro", msg.MessageReference)
			return
		}

		zap.L().Debug("content parser success")
		content := fmt.Sprintf("URL: %s\nContent:\n%s", url, parsedContent)

		llmRequest = content
	} else {
		zap.L().Info("no url found", zap.String("message", msg.Content))

		llmRequest = msg.Content
	}

	zap.L().Debug("inferencing", zap.String("content", llmRequest), zap.Any("history", history))
	err, llmResponse = client.Infer(config.Data.Model, system, llmRequest, history, ctx)

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

	if msg.GuildID == "" {
		_, err = session.ChannelMessageSend(msg.ChannelID, llmResponse)
	} else {
		_, err = session.ChannelMessageSendReply(msg.ChannelID, llmResponse, msg.Reference())
	}

	if err != nil {
		zap.L().Error("error sending reply", zap.String("text", err.Error()))
		return
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
