package main

import (
	"log"
	"os"

	gobotcfg "new_era_go/internal/gobot/config"
	"new_era_go/internal/tui"
)

func main() {
	envFile := os.Getenv("BOT_ENV_FILE")
	if envFile == "" {
		envFile = ".env"
	}
	if err := gobotcfg.LoadDotEnv(envFile); err != nil {
		log.Printf("env load warning: %v", err)
	}

	stopBot, err := startBotSidecar()
	if err != nil {
		log.Fatal(err)
	}
	defer stopBot()

	if err := tui.Run(); err != nil {
		log.Fatal(err)
	}
}
