package config

import (
	"encoding/json"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds the application configuration
type Config struct {
	RedisHost        string
	RedisPort        int
	RedisDB          int
	TwilioAccountSID string
	TwilioAuthToken  string
	TwilioFromNumber string
	Contacts         map[string]string
}

// Load loads the configuration from environment variables and .env file
func Load() *Config {
	// Load .env file if it exists
	_ = godotenv.Load()

	cfg := &Config{
		RedisHost: getEnv("REDIS_HOST", "localhost"),
		RedisPort: getEnvAsInt("REDIS_PORT", 6379),
		RedisDB:   getEnvAsInt("REDIS_DB", 0),
	}

	cfg.TwilioAccountSID = os.Getenv("TWILIO_ACCOUNT_SID")
	cfg.TwilioAuthToken = os.Getenv("TWILIO_AUTH_TOKEN")

	// Parse contacts from environment variable (JSON format expected)
	// Example: {"Joey": "+1234567890", ...}
	if rawContacts := os.Getenv("NOTIFY_CONTACTS"); rawContacts != "" {
		_ = json.Unmarshal([]byte(rawContacts), &cfg.Contacts)
	}

	rawNumbers := os.Getenv("TWILIO_PHONE_NUMBERS")
	if rawNumbers != "" {
		var numbers []string
		// Try to parse as JSON list
		if err := json.Unmarshal([]byte(rawNumbers), &numbers); err == nil && len(numbers) > 0 {
			cfg.TwilioFromNumber = numbers[0]
		} else {
			// Fallback to raw string
			cfg.TwilioFromNumber = rawNumbers
		}
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	strValue := getEnv(key, "")
	if value, err := strconv.Atoi(strValue); err == nil {
		return value
	}
	return fallback
}
