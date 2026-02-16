package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// LoadDotEnv loads KEY=VALUE pairs from file into process environment.
// Existing environment values are not overwritten.
func LoadDotEnv(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open env file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}

		key, val, ok := strings.Cut(raw, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}

		val = strings.TrimSpace(val)
		val = trimEnvQuotes(val)

		if os.Getenv(key) == "" {
			_ = os.Setenv(key, val)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan env file: %w", err)
	}

	return nil
}

func trimEnvQuotes(v string) string {
	if len(v) >= 2 {
		if (strings.HasPrefix(v, "\"") && strings.HasSuffix(v, "\"")) ||
			(strings.HasPrefix(v, "'") && strings.HasSuffix(v, "'")) {
			return v[1 : len(v)-1]
		}
	}
	return v
}
