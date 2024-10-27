package main

import (
	"context"
	"discord-military-analyst-bot/internal/bot"
	"discord-military-analyst-bot/internal/config"
	"discord-military-analyst-bot/internal/llm"
	"go.uber.org/zap"
	"os"
	"os/signal"
)

func main() {
	appCtx, cancel := context.WithCancel(context.Background())
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	config.Init()
	botInstance, messageQueue := bot.Init()

	var inferenceProvider llm.Client
	switch config.Data.Provider {
	case config.OpenAI:
		inferenceProvider = llm.NewOpenAIClient(config.Data.OpenAI.Endpoint, config.Data.OpenAI.ApiKey)
	default:
		zap.L().Panic("unknown LLM inference provider", zap.Any("provider", config.Data.Provider))
	}

	for {
		select {
		case discordMessage := <-messageQueue:
			go bot.HandleMessage(discordMessage.Message, discordMessage.Session, inferenceProvider, appCtx)
		case <-appCtx.Done():
			_ = botInstance.Close()
		case <-interrupt:
			zap.L().Info("exiting")
			cancel()
			_ = botInstance.Close()
			zap.L().Debug("done")
			return
		}
	}
}
