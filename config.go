package main

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	APIKeyEnabled     bool
	APIKeyHeader      string
	APIKey            string
	ServerPort        string
	FilesDir          string
	StaticFilesPath   string
	ServeStaticFiles  bool
	MaxUploadMemoryMB int64
	MaxUploadSizeMB   int64
	APIPrefix         string
}

func LoadConfig() (Config, error) {
	if err := loadDotEnv(".env"); err != nil {
		return Config{}, err
	}

	cfg := Config{
		APIKeyEnabled:     parseBool(getEnv("API_KEY_ENABLED", "false")),
		APIKeyHeader:      getEnv("API_KEY_HEADER", "X-API-Key"),
		APIKey:            getEnv("API_KEY", ""),
		ServerPort:        getEnv("SERVER_PORT", "9935"),
		FilesDir:          getEnv("FILES_DIR", "./files"),
		StaticFilesPath:   normalizeURLPath(getEnv("STATIC_FILES_SERVE_PATH", "/files")),
		ServeStaticFiles:  parseBool(getEnv("IS_SERVE_STATIC_FILES", "true")),
		MaxUploadMemoryMB: parseInt64(getEnv("MAX_UPLOAD_MEMORY_MB", "32"), 32),
		MaxUploadSizeMB:   parseNonNegativeInt64(getEnv("MAX_UPLOAD_SIZE_MB", "0"), 0),
		APIPrefix:         normalizeAPIPrefix(getEnv("API_PREFIX", "")),
	}

	if cfg.APIKeyEnabled && cfg.APIKey == "" {
		return Config{}, errors.New("API_KEY must be set when API_KEY_ENABLED is true")
	}

	logConfig(cfg)
	return cfg, nil
}

func loadDotEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		key, value, err := parseEnvLine(line)
		if err != nil {
			return fmt.Errorf(".env line %d: %w", lineNumber, err)
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}

	return scanner.Err()
}

func parseEnvLine(line string) (string, string, error) {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid env line")
	}

	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])
	if key == "" {
		return "", "", fmt.Errorf("missing env key")
	}

	if len(value) >= 2 {
		if value[0] == '"' && value[len(value)-1] == '"' {
			unquoted, err := strconv.Unquote(value)
			if err != nil {
				return "", "", err
			}
			value = unquoted
		} else if value[0] == '\'' && value[len(value)-1] == '\'' {
			value = value[1 : len(value)-1]
		}
	}

	return key, value, nil
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func parseBool(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "true")
}

func parseInt64(value string, defaultValue int64) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed <= 0 {
		return defaultValue
	}
	return parsed
}

func parseNonNegativeInt64(value string, defaultValue int64) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed < 0 {
		return defaultValue
	}
	return parsed
}

func normalizeAPIPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" || prefix == "/" {
		return ""
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	prefix = strings.TrimRight(prefix, "/")
	if prefix == "" {
		return ""
	}
	return prefix
}

func normalizeURLPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || path == "/" {
		return "/files"
	}
	path = "/" + strings.Trim(path, "/")
	return path
}

func logConfig(cfg Config) {
	log.Println("API key enabled:", cfg.APIKeyEnabled)
	log.Println("API key header:", cfg.APIKeyHeader)
	if cfg.APIKeyEnabled {
		log.Println("API key:", maskSecret(cfg.APIKey))
	}
	log.Println("Server port:", cfg.ServerPort)
	log.Println("Files directory:", cfg.FilesDir)
	log.Println("Static files path:", cfg.StaticFilesPath)
	log.Println("Serve static files:", cfg.ServeStaticFiles)
	log.Println("Max upload memory MB:", cfg.MaxUploadMemoryMB)
	log.Println("Max upload size MB:", cfg.MaxUploadSizeMB)
	log.Println("API prefix:", cfg.APIPrefix)
}

func maskSecret(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return "****"
	}
	return value[:2] + strings.Repeat("*", len(value)-4) + value[len(value)-2:]
}
