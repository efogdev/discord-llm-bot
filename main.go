package main

import (
	"context"
	"discord-military-analyst-bot/bot"
	"discord-military-analyst-bot/config"
	"discord-military-analyst-bot/llm-providers"
	"go.uber.org/zap"
	"os"
	"os/signal"
)

func main() {
	appCtx, cancel := context.WithCancel(context.Background())
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	config.Init()
	botInstance, messageQueue := bot.Init(config.Data.Discord)

	var inferenceProvider LLMProvider.Client
	switch config.Data.Provider {
	case config.Groq:
		inferenceProvider = LLMProvider.CreateGroqClient(config.Data.Groq.ApiKey)
	case config.Ollama:
		inferenceProvider = LLMProvider.CreateOllamaProvider(config.Data.Ollama.Endpoint)
	default:
		zap.L().Error("unknown LLM inference provider", zap.Any("provider", config.Data.Provider))
		inferenceProvider = LLMProvider.CreateNoopClient()
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
