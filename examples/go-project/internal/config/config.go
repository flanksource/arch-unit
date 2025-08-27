package config

import "log"

type Config struct {
	DatabaseURL string
	APIKey      string
}

func Load() *Config {
	// This is fine - internal package using logger
	log.Println("Loading configuration...")
	
	return &Config{
		DatabaseURL: "postgres://localhost/myapp",
		APIKey:      "secret-key",
	}
}