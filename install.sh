#!/bin/bash

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Apple Code Signing Configuration (optional)
# Set APPLE_DEVELOPER_ID environment variable to enable signing
# Example: export APPLE_DEVELOPER_ID="your-certificate-hash"

echo "=========================================="
echo "  Logdump Installation Script"
echo "=========================================="

# Detect target directory
if [ -d "$HOME/.local/bin" ]; then
    TARGET_DIR="$HOME/.local/bin"
elif [ -d "$HOME/bin" ]; then
    TARGET_DIR="$HOME/bin"
elif [ -w "/usr/local/bin" ]; then
    TARGET_DIR="/usr/local/bin"
else
    TARGET_DIR="$HOME/.local/bin"
    mkdir -p "$TARGET_DIR"
fi

echo ""
echo "Installing logdump to: $TARGET_DIR"
echo ""

# Build the binary
echo "Building logdump..."
if ! go build -o logdump .; then
    echo "Error: Failed to build logdump"
    exit 1
fi

# Sign the binary (macOS only, requires APPLE_DEVELOPER_ID)
if [[ "$OSTYPE" == "darwin"* ]] && [[ -n "$APPLE_DEVELOPER_ID" ]]; then
    echo "Signing binary with Developer ID..."
    if command -v codesign &> /dev/null; then
        codesign --force --options runtime --sign "$APPLE_DEVELOPER_ID" logdump
        if [ $? -eq 0 ]; then
            echo "Binary signed successfully"
            codesign -dv logdump 2>&1 | head -3
        else
            echo "Warning: Code signing failed, continuing without signing"
        fi
    else
        echo "Warning: codesign not found, skipping signing"
    fi
elif [[ "$OSTYPE" == "darwin"* ]]; then
    echo "Skipping code signing (set APPLE_DEVELOPER_ID to enable)"
fi

# Install binary
echo "Installing binary..."
cp logdump "$TARGET_DIR/logdump"
chmod +x "$TARGET_DIR/logdump"

# Create logs directory
LOGS_DIR="$HOME/.local/share/logdump/logs"
mkdir -p "$LOGS_DIR"

# Create default config only if it doesn't exist
CONFIG_DIR="$HOME/.config"
CONFIG_FILE="$CONFIG_DIR/logdump.yaml"
mkdir -p "$CONFIG_DIR"

if [ -f "$CONFIG_FILE" ]; then
    echo "Config already exists: $CONFIG_FILE (not overwriting)"
else
    echo "Creating default config..."
    cat > "$CONFIG_FILE" << 'EOF'
# Logdump Global Configuration
# Apps can write logs to ~/.local/share/logdump/logs/ for agent access

streams:
  - name: apps
    path: ~/.local/share/logdump/logs
    patterns:
      - "*.log"
      - "*.txt"
    tags:
      - application
    color: cyan

groups:
  - name: errors
    pattern: "ERROR|FATAL|ERR|error|fatal"
    color: red
    streams:
      - apps

  - name: warnings
    pattern: "WARN|WARNING|warn|warning"
    color: yellow
    streams:
      - apps

  - name: info
    pattern: "INFO|info"
    color: green
    streams:
      - apps
EOF
fi

# Create sample log if logs directory is empty
if [ -z "$(ls -A "$LOGS_DIR" 2>/dev/null)" ]; then
    echo "Creating sample log..."
    cat > "$LOGS_DIR/sample.log" << 'EOF'
2026-01-17 10:00:00 INFO Application initialized
2026-01-17 10:00:01 DEBUG Loading configuration
2026-01-17 10:00:02 INFO Server started on port 8080
2026-01-17 10:00:05 WARN Memory usage above 80%
2026-01-17 10:00:10 ERROR Connection to database failed
EOF
fi

echo ""
echo "=========================================="
echo "  Installation Complete!"
echo "=========================================="
echo ""
echo "Binary installed: $TARGET_DIR/logdump"
echo "Config file: $CONFIG_FILE"
echo "Logs directory: $LOGS_DIR"
echo ""
echo "Usage:"
echo "  logdump                     # Run TUI mode"
echo "  logdump -mcp                # Run MCP server mode"
echo "  logdump -config /path/to/cfg # Use custom config"
echo ""
echo "Apps can write logs to: $LOGS_DIR"
echo "MCP agents will automatically read from this directory."
echo ""
echo "Add to PATH if needed:"
echo "  export PATH=\"$TARGET_DIR:\$PATH\""
echo ""
