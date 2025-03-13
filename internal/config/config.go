package config

import (
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type LLMProvider int8

const (
	OpenAI LLMProvider = iota
)

type Environment int8

const (
	Development Environment = iota
	Production
)

type DiscordConfig struct {
	Token               string
	BotId               string
	SuperuserId         string
	BonkEmojiName       string
	BonkFromAnyone      bool
	Typing              bool
	IgnoreSystemKeyword string
	MakeImageKeyword    string
	AllowDM             bool
	DisableSystemForDM  bool
}

type OpenAIConfig struct {
	Endpoint      string
	ImageEndpoint string
	ApiKey        string
	Temperature   float64
}

type DatabaseConfig struct {
	Path string
}

type Config struct {
	Discord    DiscordConfig
	OpenAI     OpenAIConfig
	Database   DatabaseConfig
	Provider   LLMProvider
	Model      string
	ImageModel string
	LogLevel   zapcore.Level
	EnvType    Environment
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

	envString := viper.Get("APP_ENV")
	switch envString {
	case "production":
	case "prod":
		config.EnvType = Production
	default:
		config.EnvType = Development
	}

	config.Discord = DiscordConfig{
		Token:               viper.GetString("DISCORD_TOKEN"),
		BotId:               viper.GetString("DISCORD_BOT_ID"),
		SuperuserId:         viper.GetString("DISCORD_SUPERUSER_ID"),
		BonkEmojiName:       viper.GetString("DISCORD_BONK_EMOJI_NAME"),
		BonkFromAnyone:      viper.GetBool("DISCORD_BONK_FROM_ANYONE"),
		IgnoreSystemKeyword: viper.GetString("DISCORD_IGNORE_SYSTEM_KEYWORD"),
		MakeImageKeyword:    viper.GetString("DISCORD_MAKE_IMAGE_KEYWORD"),
		Typing:              viper.GetBool("DISCORD_TYPING"),
		AllowDM:             viper.GetBool("DISCORD_ALLOW_DM"),
		DisableSystemForDM:  viper.GetBool("DISCORD_DM_CLEAN_SYSTEM"),
	}

	config.OpenAI = OpenAIConfig{
		Endpoint:      viper.GetString("OPENAI_ENDPOINT"),
		ImageEndpoint: viper.GetString("OPENAI_IMG_ENDPOINT"),
		ApiKey:        viper.GetString("OPENAI_API_KEY"),
		Temperature:   viper.GetFloat64("OPENAI_TEMPERATURE"),
	}

	config.Database = DatabaseConfig{
		Path: viper.GetString("DB_PATH"),
	}

	config.Model = viper.GetString("MODEL")
	config.ImageModel = viper.GetString("IMAGE_MODEL")

	if config.Model == "" {
		zap.L().Fatal("model name is required")
	}

	if config.Discord.BotId == "" || config.Discord.Token == "" {
		zap.L().Fatal("invalid discord config")
	}

	zap.L().Debug("config loaded")
}

func InitLogger() {
	zapConfig := zap.Config{
		Level:            zap.NewAtomicLevelAt(Data.LogLevel),
		Development:      false,
		Encoding:         "json",
		EncoderConfig:    zap.NewProductionEncoderConfig(),
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	if Data.EnvType == Development {
		zapConfig.Development = true
		zapConfig.Encoding = "console"
		zapConfig.EncoderConfig = zap.NewDevelopmentEncoderConfig()
		zapConfig.EncoderConfig.TimeKey = ""
		zapConfig.EncoderConfig.LevelKey = ""
	}

	logger, _ := zapConfig.Build()
	defer zap.ReplaceGlobals(logger)
}
