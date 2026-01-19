# Homebrew formula for logdump
# This file should be placed in appgram/homebrew-tap repo under Formula/logdump.rb

class Logdump < Formula
  desc "Real-time log aggregation tool with TUI and MCP server for AI agents"
  homepage "https://github.com/appgram/logdump"
  version "1.0.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/appgram/logdump/releases/download/v#{version}/logdump_#{version}_darwin_arm64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_ARM64"
    else
      url "https://github.com/appgram/logdump/releases/download/v#{version}/logdump_#{version}_darwin_amd64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_AMD64"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/appgram/logdump/releases/download/v#{version}/logdump_#{version}_linux_arm64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_LINUX_ARM64"
    else
      url "https://github.com/appgram/logdump/releases/download/v#{version}/logdump_#{version}_linux_amd64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_LINUX_AMD64"
    end
  end

  def install
    bin.install "logdump"
  end

  def post_install
    # Create default config directory
    (etc/"logdump").mkpath
    
    # Create logs directory
    (var/"log/logdump").mkpath
  end

  def caveats
    <<~EOS
      To use logdump with Claude Code MCP, add to your settings:

        {
          "mcpServers": {
            "logdump": {
              "command": "#{opt_bin}/logdump",
              "args": ["-mcp"]
            }
          }
        }

      Default log directory: ~/.local/share/logdump/logs/
      Config file: ~/.config/logdump.yaml
    EOS
  end

  test do
    assert_match "logdump", shell_output("#{bin}/logdump -version 2>&1", 0)
  end
end
