package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"

	"github.com/appgram/logdump/internal/config"
	"github.com/appgram/logdump/internal/logtail"
	"github.com/appgram/logdump/internal/mcp"
	"github.com/appgram/logdump/internal/tui"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	printVersion := flag.Bool("version", false, "Print version and exit")
	configPath := flag.String("config", "", "Path to config file")
	mcpMode := flag.Bool("mcp", false, "Run in MCP server mode")
	mcpTransport := flag.String("mcp-transport", "stdio", "MCP transport type (stdio, websocket)")
	excludeFlag := flag.String("exclude", "", "Comma-separated list of streams to exclude (e.g., -exclude mcp-activity,sample)")
	tailOnly := flag.Bool("tail", false, "Only show new logs, don't load history")
	flag.Parse()

	if *printVersion {
		fmt.Printf("logdump version %s, commit %s, date %s\n", version, commit, date)
		os.Exit(0)
	}

	// Parse exclude list
	exclude := make(map[string]bool)
	if *excludeFlag != "" {
		for _, name := range strings.Split(*excludeFlag, ",") {
			exclude[strings.TrimSpace(name)] = true
		}
	}

	// In MCP mode, use global config for consistent agent access across all directories.
	// In TUI mode, check local configs first, then global.
	var cfg *config.Config
	var err error

	if *mcpMode && *configPath == "" {
		// MCP mode without explicit config: use global config only
		cfg, err = config.LoadGlobal()
	} else {
		// TUI mode or explicit config path: use normal loading
		cfg, err = config.Load(*configPath)
	}

	if err != nil {
		// Create empty config if loading fails
		cfg = &config.Config{
			Streams: []config.StreamConfig{},
			Groups:  []config.GroupConfig{},
		}
		if !*mcpMode {
			// In TUI mode, just warn but continue with auto-discovery
			fmt.Fprintf(os.Stderr, "Warning: %v, using auto-discovery\n", err)
		}
	}

	// Auto-discover log files
	if err := cfg.AutoDiscover(exclude); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: auto-discovery failed: %v\n", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if *mcpMode {
		runMCPServer(ctx, cfg, *mcpTransport)
		return
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		cancel()
	}()

	manager := logtail.NewManagerWithOptions(*tailOnly)

	var wg sync.WaitGroup
	for _, stream := range cfg.Streams {
		wg.Add(1)
		go func(s config.StreamConfig) {
			defer wg.Done()
			if err := manager.Tail(s); err != nil {
				fmt.Printf("Failed to tail %s: %v\n", s.Name, err)
			}
		}(stream)
	}

	p := tea.NewProgram(tui.New(manager, cfg), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatalf("UI error: %v", err)
	}
}

func runMCPServer(ctx context.Context, cfg *config.Config, transport string) {
	manager := logtail.NewManager()
	manager.StartBuffering()
	server := mcp.NewServer(manager, cfg)

	// Use stderr for logging in MCP mode to avoid corrupting JSON-RPC over stdout
	fmt.Fprintln(os.Stderr, "Starting MCP server...")

	for _, stream := range cfg.Streams {
		go func(s config.StreamConfig) {
			if err := manager.Tail(s); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to tail %s: %v\n", s.Name, err)
			}
		}(stream)
	}

	// Wait for initial file reads to be processed into buffer
	// This prevents race condition where MCP requests arrive before entries are buffered
	time.Sleep(200 * time.Millisecond)

	switch transport {
	case "stdio":
		if err := server.RunStdio(ctx); err != nil {
			log.Fatalf("MCP server error: %v", err)
		}
	case "websocket":
		if err := server.RunWebsocket(ctx, ":8765"); err != nil {
			log.Fatalf("MCP server error: %v", err)
		}
	default:
		log.Fatalf("Unknown transport: %s", transport)
	}
}
