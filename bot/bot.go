package bot

import (
	"discord-military-analyst-bot/config"
	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
)

func Init(discordConfig config.Discord) (*discordgo.Session, chan *discordgo.MessageCreate) {
	zap.L().Info("Initializing Discord bot...")

	discord, err := discordgo.New("Bot " + discordConfig.Token)
	queue := make(chan *discordgo.MessageCreate, 128)

	if err != nil {
		zap.L().Error("Incorrect Discord token.", zap.Error(err))
		return nil, nil
	}

	discord.AddHandler(func(session *discordgo.Session, message *discordgo.MessageCreate) {
		queue <- message
	})

	discord.Identify.Intents = discordgo.IntentsGuildMessages

	err = discord.Open()
	if err != nil {
		zap.L().Error("Error while initializing Discord.", zap.Error(err))
		return nil, nil
	}

	return discord, queue
}
