package config

import (
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type LLMProvider int8

const (
	Ollama LLMProvider = iota
	Groq
)

type DiscordConfig struct {
	Token string
	BotId string
}

type GroqConfig struct {
	ApiKey string
}

type Config struct {
	Discord  DiscordConfig
	Groq     GroqConfig
	Provider LLMProvider
	Model    string
}

func Init() *Config {
	config := Config{}

	viper.AddConfigPath(".")
	viper.SetConfigName("app")
	viper.SetConfigType("env")
	viper.AutomaticEnv()

	err := viper.ReadInConfig()
	if err != nil {
		zap.L().Fatal("fatal error config file", zap.Error(err))
	}

	initLogger()

	providerString := viper.Get("LLM_PROVIDER")
	switch providerString {
	case "ollama":
		config.Provider = Ollama
	case "groq":
		config.Provider = Groq
	default:
		config.Provider = Ollama
	}

	config.Discord = DiscordConfig{
		Token: viper.GetString("DISCORD_TOKEN"),
		BotId: viper.GetString("DISCORD_BOT_ID"),
	}

	config.Groq = GroqConfig{
		ApiKey: viper.GetString("GROQ_API_KEY"),
	}

	config.Model = viper.GetString("MODEL")

	return &config
}

func initLogger() {
	logger := zap.NewExample()
	defer zap.ReplaceGlobals(logger)
}
