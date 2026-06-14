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
	JWTSecret          string
	BinanceSymbols     []string
	MarketfeedEnabled  bool
	CORSAllowedOrigins []string
	SimbotEnabled      bool
	Environment        string
}

// Known environments. Anything not in this set is treated as production for
// safety — sandbox-only features stay gated unless the env explicitly opts in.
const (
	EnvProduction = "production"
	EnvSandbox    = "sandbox"
)

// IsSandbox reports whether sandbox-only features (e.g. POST /deposits/simulate)
// should be exposed. Centralised so route registration and feature gates agree.
func (c *Config) IsSandbox() bool {
	return c.Environment == EnvSandbox
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

	jwtSecret := v.GetString("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = v.GetString("jwt.secret")
	}
	if len(jwtSecret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET is required and must be at least 32 characters (got %d)", len(jwtSecret))
	}

	symbolsRaw := v.GetString("BINANCE_SYMBOLS")
	if symbolsRaw == "" {
		symbolsRaw = v.GetString("binance.symbols")
	}
	if symbolsRaw == "" {
		symbolsRaw = "BTC-USDT,ETH-USDT,SOL-USDT"
	}
	symbols := []string{}
	for _, s := range strings.Split(symbolsRaw, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			symbols = append(symbols, s)
		}
	}

	mfEnabled := true
	if v.IsSet("MARKETFEED_ENABLED") {
		mfEnabled = v.GetBool("MARKETFEED_ENABLED")
	} else if v.IsSet("marketfeed.enabled") {
		mfEnabled = v.GetBool("marketfeed.enabled")
	}

	simEnabled := false
	if v.IsSet("SIMBOT_ENABLED") {
		simEnabled = v.GetBool("SIMBOT_ENABLED")
	} else if v.IsSet("simbot.enabled") {
		simEnabled = v.GetBool("simbot.enabled")
	}

	environment := v.GetString("ENVIRONMENT")
	if environment == "" {
		environment = v.GetString("environment")
	}
	if environment == "" {
		environment = EnvProduction
	}

	corsRaw := v.GetString("CORS_ALLOWED_ORIGINS")
	if corsRaw == "" {
		corsRaw = v.GetString("cors.allowed_origins")
	}
	corsOrigins := []string{}
	for _, s := range strings.Split(corsRaw, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			corsOrigins = append(corsOrigins, s)
		}
	}
	if len(corsOrigins) == 0 {
		return nil, fmt.Errorf("cors.allowed_origins is required: set CORS_ALLOWED_ORIGINS env var or cors.allowed_origins in application.yaml")
	}

	return &Config{
		DatabaseURL:        dbURL,
		ServerPort:         v.GetString("server.port"),
		LogLevel:           v.GetString("log.level"),
		DBMaxConns:         v.GetInt("db.max_conns"),
		OutboxPollInterval: time.Duration(v.GetInt("outbox.poll_interval_seconds")) * time.Second,
		JWTSecret:          jwtSecret,
		BinanceSymbols:     symbols,
		MarketfeedEnabled:  mfEnabled,
		CORSAllowedOrigins: corsOrigins,
		SimbotEnabled:      simEnabled,
		Environment:        environment,
	}, nil
}
