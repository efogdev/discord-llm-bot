package main

import (
	"context"
	"discord-military-analyst-bot/bot"
	"discord-military-analyst-bot/config"
	"go.uber.org/zap"
	"os"
	"os/signal"
)

func main() {
	appCtx, cancel := context.WithCancel(context.Background())
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	defer func() {
		signal.Stop(interrupt)
		cancel()
	}()

	configData := config.Init()
	discordBot, messageQueue := bot.Init(configData.Discord)

	go func() {
		select {
		case <-appCtx.Done():
			discordBot.Close()
		case <-interrupt:
			zap.L().Info("Done.")
			cancel()
		}
	}()

	for {
		select {
		case message := <-messageQueue:
			{
				zap.L().Info("Message received.", zap.String("text", message.Content))
			}
		}
	}
}
