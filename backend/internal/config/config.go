package config

import (
	"fmt"
	"os"
)

type Config struct {
	HTTPPort       string
	FrontendOrigin string
	PostgresDSN    string
}

func Load() Config {
	return Config{
		HTTPPort:       getEnv("HTTP_PORT", "8080"),
		FrontendOrigin: getEnv("FRONTEND_ORIGIN", "http://localhost:5173"),
		PostgresDSN: getEnv(
			"POSTGRES_DSN",
			"host=localhost port=5432 user=aiops_user password=aiops_pass dbname=aiops sslmode=disable",
		),
	}
}

func (config Config) HTTPAddress() string {
	return fmt.Sprintf(":%s", config.HTTPPort)
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
