# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.1] - 2026-01-19

### Added
- ASCII art splash screen on startup

## [1.0.0] - 2026-01-18

### Added
- Real-time log tailing from multiple files
- Beautiful TUI with Bubble Tea framework
- MCP (Model Context Protocol) server for AI agent integration
- Auto-discovery of `.log` and `.txt` files in log directory
- Stream filtering with toggle on/off
- Regex search in logs
- Reverse order mode (newest at top/bottom)
- Detail view for log entries
- Stream list overlay for managing many streams
- Color-coded log output based on stream
- Keyboard navigation (vim-style)
- `-tail` flag to skip loading history
- `-exclude` flag to exclude specific streams
- Code signing for macOS binaries
- Homebrew tap support

### MCP Tools
- `logdump_read` - Read log entries with optional filtering
- `logdump_grep` - Search logs with regex patterns
- `logdump_streams` - List active log streams
- `logdump_groups` - List log groups
- `logdump_create_group` - Create custom log groups
- `logdump_stats` - Get buffer and stream statistics
- `logdump_access_log` - View agent access history

### Configuration
- YAML config file support (`~/.config/logdump.yaml`)
- Global log directory (`~/.local/share/logdump/logs/`)
- Custom stream definitions with patterns and colors
- Log groups with regex patterns

[1.0.1]: https://github.com/appgram/logdump/releases/tag/v1.0.1
[1.0.0]: https://github.com/appgram/logdump/releases/tag/v1.0.0
