package env

import (
	"os"
	"strconv"
)

type Env = struct {
	// Set DB_MIGRATE_ON_START=true (or 1, t, etc.) to enable. Defaults to false.
	DbMigrateOnStart bool
	Host             string
	Port             string
	// For logging purposes only. Not needed in production, so they're optional.
	LiveReloadHost string
	// For logging purposes only. Not needed in production, so they're optional.
	LiveReloadPort string
	// Chat model used for LLM completions. Defaults to "gpt-4o-mini".
	OpenAIModel string
	// Base URL of the OpenAI-compatible chat completions API. Defaults to
	// "http://localhost:6700" (a local llama.cpp/llama-server instance) --
	// point this at OpenAI's own API, or any other compatible server.
	OpenAIBaseURL string
	// Path to the sqlite database file. Defaults to "./data/sqlite.db".
	DBPath string
	// Set COOKIE_SECURE=true (or 1, t, etc.) when serving over HTTPS, so the
	// session cookie is marked Secure. Defaults to false for plain-HTTP
	// local/dev deployments.
	CookieSecure bool
}

var Vars *Env

func init() {
	migrate, _ := strconv.ParseBool(os.Getenv("DB_MIGRATE_ON_START"))
	cookieSecure, _ := strconv.ParseBool(os.Getenv("COOKIE_SECURE"))
	Vars = &Env{
		DbMigrateOnStart: migrate,
		CookieSecure:     cookieSecure,
		Host:             GetEnv("HOST", "localhost"),
		Port:             GetEnv("PORT", "8080"),
		LiveReloadHost:   os.Getenv("LIVE_RELOAD_PROXY_HOST"),
		LiveReloadPort:   os.Getenv("LIVE_RELOAD_PROXY_PORT"),
		OpenAIModel:      GetEnv("OPENAI_MODEL", "gpt-4o-mini"),
		OpenAIBaseURL:    GetEnv("OPENAI_BASE_URL", "http://localhost:6700"),
		DBPath:           GetEnv("DB_PATH", "./data/sqlite.db"),
	}
}

func GetEnv(name string, fallback string) string {
	if val := os.Getenv(name); val != "" {
		return val
	}
	return fallback
}
