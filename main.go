package main

import (
	"context"
	"discord-military-analyst-bot/bot"
	"discord-military-analyst-bot/config"
	"os"
	"os/signal"
)

func main() {
	appCtx, cancel := context.WithCancel(context.Background())
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	configData := config.Init()
	discordBot := bot.Init(configData.Discord)

	for {
		select {
		case <-appCtx.Done():
			err := discordBot.Close()
			if err != nil {
				return
			}
		case <-interrupt:
			err := discordBot.Close()
			if err != nil {
				return
			}
			cancel()
		}
	}
}
