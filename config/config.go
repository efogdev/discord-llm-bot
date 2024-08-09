package config

import (
	"fmt"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type LLMProvider int8

const (
	Ollama LLMProvider = iota
	Groq
)

type Discord struct {
	Token       string
	WorkspaceId string
	BotId       string
}

type Config struct {
	Discord  Discord
	Provider LLMProvider
	Logger   *zap.Logger
}

func Init() *Config {
	config := Config{}

	viper.AddConfigPath(".")
	viper.SetConfigName("app")
	viper.SetConfigType("env")
	viper.AutomaticEnv()

	err := viper.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("fatal error config file: %w", err))
	}

	config.Logger = initLogger()

	providerString := viper.Get("LLM_PROVIDER")
	switch providerString {
	case "ollama":
		config.Provider = Ollama
	case "groq":
		config.Provider = Groq
	default:
		config.Provider = Ollama
	}

	config.Discord = Discord{
		Token:       viper.GetString("DISCORD_TOKEN"),
		WorkspaceId: viper.GetString("DISCORD_WORKSPACE_ID"),
		BotId:       viper.GetString("DISCORD_BOT_ID"),
	}

	return &config
}

func initLogger() *zap.Logger {
	logger := zap.NewExample()
	defer logger.Sync()
	defer zap.ReplaceGlobals(logger)

	return logger
}
