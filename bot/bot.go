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

func Init(discordConfig config.DiscordConfig) (*discordgo.Session, chan *DiscordMessage) {
	zap.L().Debug("initializing bot")

	discord, err := discordgo.New("Bot " + discordConfig.Token)
	queue := make(chan *DiscordMessage, 128)

	if err != nil {
		zap.L().Error("incorrect Discord token", zap.Error(err))
		return nil, nil
	}

	discord.AddHandler(func(session *discordgo.Session, message *discordgo.MessageCreate) {
		if message.Author.ID == discordConfig.BotId {
			return
		}

		queue <- &DiscordMessage{session, message}
	})

	discord.AddHandler(func(session *discordgo.Session, reaction *discordgo.MessageReactionAdd) {
		if reaction.Emoji.Name == discordConfig.BonkEmojiName && (reaction.Member.User.ID == discordConfig.SuperuserId || discordConfig.BonkFromAnyone) {
			err = session.ChannelMessageDelete(reaction.MessageReaction.ChannelID, reaction.MessageReaction.MessageID)
			zap.L().Info("got bonk, removing message", zap.Any("reaction", reaction))

			if err != nil {
				zap.L().Error("error deleting message", zap.Error(err))
			}
		}
	})

	discord.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsGuildMessageReactions

	err = discord.Open()
	if err != nil {
		zap.L().Error("error initializing Discord bot", zap.Error(err))
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
	if err != nil {
		return
	}

	zap.L().Debug("message received", zap.String("text", msg.Content))

	ignoreSystemPrompt := false

	if config.Data.Discord.IgnoreSystemKeyword != "" {
		if strings.Contains(msg.Content, config.Data.Discord.IgnoreSystemKeyword) {
			ignoreSystemPrompt = true
		}

		for _, item := range history {
			if !item.IsBot && strings.Contains(item.Content, config.Data.Discord.IgnoreSystemKeyword) {
				ignoreSystemPrompt = true
			}
		}
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
		zap.L().Fatal("error reading prompt file", zap.Error(err))
	}

	var llmRequest string
	var llmResponse string
	if len(history) <= 1 && url != "" {
		zap.L().Info("found url to parse", zap.String("url", url))

		if ignoreSystemPrompt {
			sanitizedContent := strings.ReplaceAll(msg.Content, config.Data.Discord.IgnoreSystemKeyword, "")
			sanitizedContent = strings.ReplaceAll(sanitizedContent, url, "")

			history = append(history, LLMProvider.HistoryItem{
				Content: sanitizedContent,
				IsBot:   false,
			})
		}

		_ = session.MessageReactionAdd(msg.ChannelID, msg.ID, "👀")

		err, parsedContent := ParseURL(url)
		if err != nil {
			zap.L().Warn("couldn't parse the url")
			_, _ = session.ChannelMessageSendReply(msg.ChannelID, "Your link is bullshit bro.", msg.MessageReference)
			return
		}

		zap.L().Info("content parser success")
		content := fmt.Sprintf("URL: %s\nContent:\n%s", url, parsedContent)

		llmRequest = content
	} else {
		zap.L().Info("no url found, running simple dialog", zap.String("message", msg.Content))

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

	_, err = session.ChannelMessageSendReply(msg.ChannelID, llmResponse, msg.Reference())
	if err != nil {
		zap.L().Warn("error sending reply", zap.String("text", err.Error()))
		return
	}
}

func ParseURL(url string) (error, string) {
	cmd := exec.Command("bun", "run", "index.js", url)
	cmd.Dir = filepath.Join(".", "content-from-webpage")

	output, err := cmd.Output()
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			if exitError.ExitCode() != 0 {
				zap.L().Error("content parser failed", zap.Int("errorCode", exitError.ExitCode()))
			}
		}

		return err, ""
	}

	return nil, strings.TrimSpace(string(output))
}
