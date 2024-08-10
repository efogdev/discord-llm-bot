package main

import (
	"context"
	"discord-military-analyst-bot/bot"
	"discord-military-analyst-bot/config"
	"discord-military-analyst-bot/config/llm_providers"
	"go.uber.org/zap"
	"os"
	"os/signal"
	"path/filepath"
)

func main() {
	appCtx, cancel := context.WithCancel(context.Background())
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	configData := config.Init()
	discordBot, messageQueue := bot.Init(configData.Discord)

	var inferenceProvider llm_providers.LLMProviderClient
	switch configData.Provider {
	case config.Groq:
		inferenceProvider = llm_providers.CreateGroqProvider(configData.Groq.ApiKey)
	case config.Ollama:
		inferenceProvider = llm_providers.CreateNoopLLMProviderClient()
	}

	promptFilePath := filepath.Join("config", "prompt.txt")
	systemPrompt, err := os.ReadFile(promptFilePath)
	if err != nil {
		zap.L().Fatal("error reading prompt file", zap.Error(err))
		return
	}

	inferenceProvider.SetSystem(string(systemPrompt))

	for {
		select {
		case discordMessage := <-messageQueue:
			zap.L().Info("message received", zap.String("text", discordMessage.Message.Content))

			go bot.HandleMessage(
				discordMessage.Message,
				discordMessage.Session,
				inferenceProvider,
				configData.Model,
				configData.Discord.BotId,
			)
		case <-appCtx.Done():
			_ = discordBot.Close()
		case <-interrupt:
			zap.L().Info("exiting...")
			cancel()
			_ = discordBot.Close()
			zap.L().Info("done")
			os.Exit(0)
		}
	}
}
