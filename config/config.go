package config

import (
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
)

type LLMProvider int8

const (
	OpenAI LLMProvider = iota
)

type DiscordConfig struct {
	Token                 string
	BotId                 string
	SuperuserId           string
	BonkEmojiName         string
	BonkFromAnyone        bool
	Typing                bool
	KeywordToIgnoreSystem string
	AllowDM               bool
	DisableSystemForDM    bool
}

type OpenAIConfig struct {
	Endpoint string
	ApiKey   string
}

type Config struct {
	Discord  DiscordConfig
	OpenAI   OpenAIConfig
	Provider LLMProvider
	Model    string
	LogLevel zapcore.Level
}

var Data *Config = nil

func Init() {
	config := Config{}
	Data = &config

	viper.AddConfigPath(".")
	viper.SetConfigName("app")
	viper.SetConfigType("env")
	viper.AutomaticEnv()

	err := viper.ReadInConfig()
	if err != nil {
		config.LogLevel = zapcore.DebugLevel

		InitLogger()
		zap.L().Fatal("error reading config file", zap.Error(err))
	}

	levelString := viper.GetString("LOG_LEVEL")
	switch levelString {
	case "debug":
		config.LogLevel = zapcore.DebugLevel
	case "info":
		config.LogLevel = zapcore.InfoLevel
	case "warn":
		config.LogLevel = zapcore.WarnLevel
	case "error":
		config.LogLevel = zapcore.ErrorLevel
	default:
		config.LogLevel = zapcore.InfoLevel
	}

	InitLogger()

	providerString := viper.Get("LLM_PROVIDER")
	switch providerString {
	case "openai":
		config.Provider = OpenAI
	default:
		config.Provider = OpenAI
	}

	config.Discord = DiscordConfig{
		Token:                 viper.GetString("DISCORD_TOKEN"),
		BotId:                 viper.GetString("DISCORD_BOT_ID"),
		SuperuserId:           viper.GetString("DISCORD_SUPERUSER_ID"),
		BonkEmojiName:         viper.GetString("DISCORD_BONK_EMOJI_NAME"),
		BonkFromAnyone:        viper.GetBool("DISCORD_BONK_FROM_ANYONE"),
		KeywordToIgnoreSystem: viper.GetString("DISCORD_IGNORE_SYSTEM_KEYWORD"),
		Typing:                viper.GetBool("DISCORD_TYPING"),
		AllowDM:               viper.GetBool("DISCORD_ALLOW_DM"),
		DisableSystemForDM:    viper.GetBool("DISCORD_DM_CLEAN_SYSTEM"),
	}

	config.OpenAI = OpenAIConfig{
		Endpoint: viper.GetString("OPENAI_ENDPOINT"),
		ApiKey:   viper.GetString("OPENAI_API_KEY"),
	}

	config.Model = viper.GetString("MODEL")

	if config.Model == "" {
		zap.L().Fatal("model name is required")
	}

	if config.Discord.BotId == "" || config.Discord.Token == "" {
		zap.L().Fatal("invalid discord config")
	}

	zap.L().Debug("config loaded")
}

func InitLogger() {
	encoderCfg := zapcore.EncoderConfig{
		MessageKey:     "msg",
		LevelKey:       "level",
		NameKey:        "logger",
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
	}
	core := zapcore.NewCore(zapcore.NewJSONEncoder(encoderCfg), os.Stdout, Data.LogLevel)
	logger := zap.New(core)

	defer zap.ReplaceGlobals(logger)
}
