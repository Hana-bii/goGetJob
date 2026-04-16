package main

import (
	"fmt"
	"os"
	"path/filepath"

	"goGetJob/internal/app"
	"goGetJob/internal/common/config"
	"goGetJob/internal/common/logger"
)

func main() {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = filepath.Join("configs", "config.example.yaml")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	log := logger.New(cfg.App.Env)
	engine := app.New(cfg, log)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Info("starting server", "addr", addr, "config_path", configPath)

	if err := engine.Run(addr); err != nil {
		log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
