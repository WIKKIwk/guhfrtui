package main

import (
	"log"

	"new_era_go/internal/tui"
)

func main() {
	if err := tui.Run(); err != nil {
		log.Fatal(err)
	}
}
