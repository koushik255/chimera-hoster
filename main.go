package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

type logWriter struct{}

func (l *logWriter) Printf(format string, args ...any) {
	log.Printf(format, args...)
}

func main() {
	configPath := flag.String("config", defaultConfigPath(), "Path to the host config file")
	checkOnly := flag.Bool("check", false, "Validate config and scan the library, then exit")
	dumpManifest := flag.Bool("dump-manifest", false, "Print the generated manifest JSON, then exit")
	flag.Parse()

	config, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("load host config: %v", err)
	}

	if *checkOnly {
		_, _, summary, err := buildManifest(config)
		if err != nil {
			log.Fatalf("scan host library: %v", err)
		}

		fmt.Printf("Config: %s\n", config.Path)
		fmt.Printf("Backend: %s\n", config.WSURL)
		fmt.Printf("Host: %s (%s)\n", config.Host.Username, config.Host.ID)
		fmt.Printf("Series paths: %d\n", len(config.SeriesPaths))
		for _, seriesPath := range config.SeriesPaths {
			fmt.Printf("  - %s\n", seriesPath)
		}
		fmt.Printf("Library: %d series, %d volumes, %d pages\n", summary.Series, summary.Volumes, summary.Pages)
		return
	}

	if *dumpManifest {
		manifest, _, _, err := buildManifest(config)
		if err != nil {
			log.Fatalf("build manifest: %v", err)
		}
		manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			log.Fatalf("encode manifest: %v", err)
		}
		fmt.Println(string(manifestJSON))
		return
	}

	log.Printf("Using config: %s", config.Path)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger := &logWriter{}
	if err := runHost(ctx, config, logger); err != nil && err != context.Canceled {
		log.Printf("host stopped with error: %v", err)
		os.Exit(1)
	}
}

func defaultConfigPath() string {
	candidates := []string{
		"config.json",
		"../hoster/config.json",
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return filepath.Join("config.json")
}
