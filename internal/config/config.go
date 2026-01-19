package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	LogDir  string         `yaml:"log_dir"`  // Directory for auto-discovery
	Streams []StreamConfig `yaml:"streams"`
	Theme   ThemeConfig    `yaml:"theme"`
	Filters []FilterConfig `yaml:"filters"`
	Groups  []GroupConfig  `yaml:"groups"`
}

type GroupConfig struct {
	Name    string   `yaml:"name"`
	Pattern string   `yaml:"pattern"`
	Color   string   `yaml:"color"`
	Streams []string `yaml:"streams"`
}

type StreamConfig struct {
	Name     string   `yaml:"name"`
	Path     string   `yaml:"path"`
	Patterns []string `yaml:"patterns"`
	Tags     []string `yaml:"tags"`
	Color    string   `yaml:"color"`
}

type ThemeConfig struct {
	Background string `yaml:"background"`
	Foreground string `yaml:"foreground"`
	Accent     string `yaml:"accent"`
}

type FilterConfig struct {
	Name    string   `yaml:"name"`
	Pattern string   `yaml:"pattern"`
	Color   string   `yaml:"color"`
	Actions []string `yaml:"actions"`
}

func Load(path string) (*Config, error) {
	return LoadWithOptions(path, false)
}

// LoadGlobal loads only the global config (for MCP mode)
func LoadGlobal() (*Config, error) {
	return LoadWithOptions("", true)
}

func LoadWithOptions(path string, globalOnly bool) (*Config, error) {
	if path == "" {
		path = FindConfigFile(globalOnly)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Expand ~ in stream paths
	for i := range cfg.Streams {
		cfg.Streams[i].Path = expandPath(cfg.Streams[i].Path)
	}

	return &cfg, nil
}

// expandPath expands ~ to the user's home directory
func expandPath(path string) string {
	if len(path) == 0 {
		return path
	}
	if path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[1:])
	}
	return path
}

// FindConfigFile locates the config file. If globalOnly is true, only checks global config location.
func FindConfigFile(globalOnly bool) string {
	var locations []string

	if globalOnly {
		// MCP mode: only use global config for consistent agent access
		locations = []string{
			filepath.Join(os.Getenv("HOME"), ".config", "logdump.yaml"),
			filepath.Join(os.Getenv("HOME"), ".config", "logdump.yml"),
		}
	} else {
		// TUI mode: check local configs first, then global
		locations = []string{
			"logdump.yaml",
			"logdump.yml",
			".logdump.yaml",
			".logdump.yml",
			filepath.Join(os.Getenv("HOME"), ".config", "logdump.yaml"),
			filepath.Join(os.Getenv("HOME"), ".config", "logdump.yml"),
		}
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			return loc
		}
	}

	return ""
}

func (c *StreamConfig) Matches(path string) bool {
	for _, pattern := range c.Patterns {
		matched, err := filepath.Match(pattern, filepath.Base(path))
		if err == nil && matched {
			return true
		}
	}
	return false
}

// colors for auto-discovered streams
var streamColors = []string{"cyan", "green", "yellow", "magenta", "blue", "red"}

// AutoDiscover scans the log directory and creates a stream for each log file.
// If exclude is provided, those stream names will be skipped.
func (cfg *Config) AutoDiscover(exclude map[string]bool) error {
	logDir := cfg.LogDir
	if logDir == "" {
		home, _ := os.UserHomeDir()
		logDir = filepath.Join(home, ".local", "share", "logdump", "logs")
	} else {
		logDir = expandPath(logDir)
	}

	// Check if directory exists
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		return nil // No log directory, no streams to discover
	}

	// Find all .log and .txt files
	files, err := filepath.Glob(filepath.Join(logDir, "*.log"))
	if err != nil {
		return err
	}
	txtFiles, _ := filepath.Glob(filepath.Join(logDir, "*.txt"))
	files = append(files, txtFiles...)

	// Create a stream for each file
	existingStreams := make(map[string]bool)
	for _, s := range cfg.Streams {
		existingStreams[s.Name] = true
	}

	colorIdx := len(cfg.Streams)
	for _, file := range files {
		base := filepath.Base(file)
		name := base[:len(base)-len(filepath.Ext(base))] // Remove extension

		// Skip if excluded or already defined
		if exclude[name] || existingStreams[name] {
			continue
		}

		cfg.Streams = append(cfg.Streams, StreamConfig{
			Name:     name,
			Path:     logDir,
			Patterns: []string{base},
			Color:    streamColors[colorIdx%len(streamColors)],
		})
		colorIdx++
	}

	return nil
}

// DefaultLogDir returns the default log directory path
func DefaultLogDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "logdump", "logs")
}
