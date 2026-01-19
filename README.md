# Logdump

Real-time log aggregation tool with TUI and MCP server for AI agents.

[![Go Report Card](https://goreportcard.com/badge/github.com/appgram/logdump)](https://goreportcard.com/report/github.com/appgram/logdump)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## Features

- **Real-time log tailing** - Watch multiple log files simultaneously
- **Beautiful TUI** - Terminal UI with color-coded streams, search, and navigation
- **MCP Server** - Expose logs to AI agents via Model Context Protocol
- **Auto-discovery** - Automatically finds `.log` and `.txt` files in your log directory
- **Stream filtering** - Toggle streams on/off, search with regex
- **Reverse mode** - View newest logs at top or bottom
- **Code signed** - macOS binary is signed with Developer ID

## Installation

### Homebrew (macOS)

```bash
brew tap appgram/tap
brew install logdump
```

### From Source

```bash
git clone https://github.com/appgram/logdump.git
cd logdump
./install.sh
```

### Go Install

```bash
go install github.com/appgram/logdump@latest
```

## Usage

### TUI Mode

```bash
# Run with auto-discovery of log files
logdump

# Only show new logs (skip history)
logdump -tail

# Exclude specific streams
logdump -exclude mcp-activity,sample

# Use custom config
logdump -config /path/to/config.yaml
```

### MCP Server Mode

```bash
# Start MCP server for AI agents
logdump -mcp
```

### TUI Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `↑/↓` or `j/k` | Navigate log entries |
| `Enter` | View log detail |
| `/` | Search (regex) |
| `s` | Show all streams |
| `1-9` | Toggle stream on/off |
| `a` | Select all streams |
| `n` | Deselect all streams |
| `r` | Reverse order (newest top/bottom) |
| `p` or `Space` | Pause/resume |
| `c` | Clear logs |
| `g` / `G` | Go to top / bottom |
| `q` | Quit |

## Configuration

Logdump uses a YAML config file located at `~/.config/logdump.yaml`:

```yaml
# Log directory for auto-discovery
log_dir: ~/.local/share/logdump/logs

# Manual stream definitions (optional)
streams:
  - name: myapp
    path: /var/log/myapp
    patterns:
      - "*.log"
    color: cyan

# Log groups for filtering
groups:
  - name: errors
    pattern: "ERROR|FATAL|ERR"
    color: red
```

### Stream Colors

Available colors: `red`, `green`, `blue`, `yellow`, `cyan`, `magenta`, `white`

## MCP Integration

Logdump exposes logs to AI agents via the [Model Context Protocol](https://modelcontextprotocol.io/).

### Claude Code Setup

Add to your Claude Code MCP settings:

```json
{
  "mcpServers": {
    "logdump": {
      "command": "logdump",
      "args": ["-mcp"]
    }
  }
}
```

### Available MCP Tools

| Tool | Description |
|------|-------------|
| `logdump_read` | Read log entries (with optional source/group filter) |
| `logdump_grep` | Search logs with regex pattern |
| `logdump_streams` | List all active log streams |
| `logdump_groups` | List log groups |
| `logdump_create_group` | Create a new log group |
| `logdump_stats` | Get buffer and stream statistics |
| `logdump_access_log` | View agent access history |

### Writing Logs for Agents

Applications can write logs to the shared directory for agent access:

```bash
# Default log directory
~/.local/share/logdump/logs/

# Example: write from your app
echo "[$(date)] INFO Starting service" >> ~/.local/share/logdump/logs/myapp.log
```

## Building

```bash
# Build
make build

# Build and sign (macOS)
make build-signed

# Install locally
make install

# Run tests
make test
```

## Requirements

- Go 1.21+
- macOS, Linux, or Windows

## License

MIT License - see [LICENSE](LICENSE)

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.
