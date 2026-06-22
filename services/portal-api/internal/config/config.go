package config

import "os"

type Config struct {
	Port        string
	DatabaseURL string
	CORSOrigin  string
}

func Load() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://platform:platform@localhost:5432/platform?sslmode=disable"
	}

	cors := os.Getenv("CORS_ORIGIN")
	if cors == "" {
		cors = "http://localhost:5173"
	}

	return Config{
		Port:        port,
		DatabaseURL: dbURL,
		CORSOrigin:  cors,
	}
}
