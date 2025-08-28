package main

import (
	"fmt"
	"log"

	"github.com/flanksource/arch-unit/examples/go-project/internal/config" // VIOLATION: accessing internal package
	"github.com/flanksource/arch-unit/examples/go-project/pkg/api"
	"github.com/flanksource/arch-unit/examples/go-project/pkg/service"
)

func main() {
	// VIOLATION: Using fmt.Println instead of logger
	fmt.Println("Starting application...")

	// This is fine - using logger
	log.Println("Application started")

	// VIOLATION: Accessing internal package
	_ = config.Load()

	// This is fine - using public packages
	server := api.NewServer()
	_ = service.NewService()

	server.Start()
}
