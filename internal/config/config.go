package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	AppEnv      string        `mapstructure:"APP_ENV"`
	LogLevel    string        `mapstructure:"LOG_LEVEL"`
	Port        int           `mapstructure:"PORT"`
	NodeID      string        `mapstructure:"NODE_ID"`
	DialTimeout time.Duration `mapstructure:"DIAL_TIMEOUT"`
	SyncTimeout time.Duration `mapstructure:"SYNC_TIMEOUT"`
	MaxWatchers int           `mapstructure:"MAX_WATCHERS"`
	ApiKey      string        `mapstructure:"API_KEY"`
	RateLimit    float64       `mapstructure:"RATE_LIMIT"`
	RateBurst    int           `mapstructure:"RATE_BURST"`
	ReadQuorum   int           `mapstructure:"READ_QUORUM"`
	WriteQuorum  int           `mapstructure:"WRITE_QUORUM"`
	ReplicaCount int           `mapstructure:"REPLICA_COUNT"`
}

func LoadConfig() (*Config, error) {
	viper.SetDefault("APP_ENV", "development")
	viper.SetDefault("LOG_LEVEL", "info")
	viper.SetDefault("PORT", 8080)
	viper.SetDefault("NODE_ID", "node-1")
	viper.SetDefault("DIAL_TIMEOUT", 2*time.Second)
	viper.SetDefault("SYNC_TIMEOUT", 5*time.Second)
	viper.SetDefault("MAX_WATCHERS", 10000)
	viper.SetDefault("API_KEY", "")
	viper.SetDefault("RATE_LIMIT", 100.0)
	viper.SetDefault("RATE_BURST", 200)
	viper.SetDefault("READ_QUORUM", 1)
	viper.SetDefault("WRITE_QUORUM", 1)
	viper.SetDefault("REPLICA_COUNT", 3)

	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
