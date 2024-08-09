package bot

import (
	"discord-military-analyst-bot/config"
	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
)

func Init(discordConfig config.Discord) *discordgo.Session {
	zap.L().Info("Initializing Discord bot...")

	discord, err := discordgo.New("Bot " + discordConfig.Token)

	if err != nil {
		zap.L().Error("Incorrect Discord token.", zap.Error(err))
		return nil
	}

	discord.AddHandler(messageReceived)
	discord.Identify.Intents = discordgo.IntentsGuildMessages

	err = discord.Open()
	if err != nil {
		zap.L().Error("Error while initializing Discord.", zap.Error(err))
		return nil
	}

	return discord
}

func messageReceived(session *discordgo.Session, message *discordgo.MessageCreate) {
	zap.L().Info("Message received.", zap.String("text", message.Content))
}
