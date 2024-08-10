package bot

import (
	"discord-military-analyst-bot/config"
	"discord-military-analyst-bot/config/llm_providers"
	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
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
	zap.L().Info("initializing Discord bot...")

	discord, err := discordgo.New("Bot " + discordConfig.Token)
	queue := make(chan *DiscordMessage, 128)

	if err != nil {
		zap.L().Error("incorrect DiscordConfig token", zap.Error(err))
		return nil, nil
	}

	discord.AddHandler(func(session *discordgo.Session, message *discordgo.MessageCreate) {
		queue <- &DiscordMessage{session, message}
	})

	discord.Identify.Intents = discordgo.IntentsGuildMessages

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

func HandleMessage(
	msg *discordgo.MessageCreate,
	session *discordgo.Session,
	client llm_providers.LLMProviderClient,
	model string,
	botId string,
) {
	url := ""

	botMentioned := false
	for _, mention := range msg.Mentions {
		if mention.ID == botId {
			botMentioned = true
			break
		}
	}

	if !botMentioned {
		return
	}

	url = FindURL(msg.Content)

	if url == "" && msg.ReferencedMessage != nil {
		url = FindURL(msg.ReferencedMessage.Content)
	}

	if url == "" {
		return
	}

	parsedContent := ParseURL(url)

	if parsedContent == "" {
		_, _ = session.ChannelMessageSendReply(msg.ChannelID, "Your link is bullshit bro.", msg.MessageReference)
		return
	}

	zap.L().Info("parser response received", zap.String("text", parsedContent))
	llmResponse := client.Infer(model, parsedContent)

	zap.L().Info("sending reply...", zap.String("text", llmResponse))
	_, err := session.ChannelMessageSendReply(msg.ChannelID, llmResponse, msg.Reference())
	if err != nil {
		zap.L().Warn("error sending reply", zap.String("text", err.Error()))
		return
	}
}

func ParseURL(url string) string {
	cmd := exec.Command("bun", "run", "index.js", url)
	cmd.Dir = filepath.Join(".", "content-from-webpage")

	output, err := cmd.Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			if exitError.ExitCode() != 0 {
				zap.L().Error("content parser failed", zap.Int("errorCode", exitError.ExitCode()))
				return ""
			}
		}

		return ""
	}

	return strings.TrimSpace(string(output))
}
