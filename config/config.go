package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	DatabaseURL        string
	ServerPort         string
	LogLevel           string
	DBMaxConns         int
	OutboxPollInterval time.Duration
}

func Load() (*Config, error) {
	v := viper.New()

	v.SetConfigFile("config/application.yaml")
	v.SetConfigType("yaml")
	_ = v.ReadInConfig() // not fatal — env vars alone are sufficient

	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// env var DATABASE_URL overrides yaml database.url
	dbURL := v.GetString("DATABASE_URL")
	if dbURL == "" {
		dbURL = v.GetString("database.url")
	}
	if dbURL == "" {
		return nil, fmt.Errorf("database URL is required: set DATABASE_URL env var or database.url in application.yaml")
	}

	return &Config{
		DatabaseURL:        dbURL,
		ServerPort:         v.GetString("server.port"),
		LogLevel:           v.GetString("log.level"),
		DBMaxConns:         v.GetInt("db.max_conns"),
		OutboxPollInterval: time.Duration(v.GetInt("outbox.poll_interval_seconds")) * time.Second,
	}, nil
}
