# Logdump AI Agent Prompt

Use this prompt when working with AI agents to help them understand and use logdump:

---

## Logdump - AI Agent Integration Guide

You have access to **logdump**, a log streaming and viewing tool. Here's how to use it effectively:

### Starting the Server

```bash
logdump -mcp -config logdump.yaml
```

### Setting Your Identity

Always set your agent identity first:

```json
{
  "method": "logdump/set_agent",
  "params": {
    "agent_id": "unique-agent-id",
    "agent_name": "Your Name"
  },
  "id": 1
}
```

### Available Tools

#### 1. **logdump_streams** - List all log streams
```json
{
  "method": "tools/call",
  "params": {
    "name": "logdump_streams",
    "arguments": {}
  }
}
```

#### 2. **logdump_groups** - List log groups
```json
{
  "method": "tools/call",
  "params": {
    "name": "logdump_groups",
    "arguments": {}
  }
}
```

#### 3. **logdump_read** - Read log entries
```json
{
  "method": "tools/call",
  "params": {
    "name": "logdump_read",
    "arguments": {
      "source": "app",        // optional: filter by stream name
      "group": "errors",      // optional: filter by group name
      "limit": 100            // optional: max entries (default 100)
    }
  }
}
```

#### 4. **logdump_grep** - Search logs with regex
```json
{
  "method": "tools/call",
  "params": {
    "name": "logdump_grep",
    "arguments": {
      "pattern": "ERROR.*connection",
      "source": "app",              // optional: filter by stream name
      "group": "errors",            // optional: filter by group name
      "limit": 50,                  // optional
      "case_insensitive": true      // optional
    }
  }
}
```

#### 5. **logdump_create_group** - Create a new log group
```json
{
  "method": "tools/call",
  "params": {
    "name": "logdump_create_group",
    "arguments": {
      "name": "database-errors",
      "pattern": "ERROR.*database|connection.*failed",
      "color": "red",
      "streams": "app,api,database"
    }
  }
}
```

#### 6. **logdump_access_log** - View agent access history
```json
{
  "method": "tools/call",
  "params": {
    "name": "logdump_access_log",
    "arguments": {
      "agent": "Claude",      // optional: filter by agent
      "limit": 50             // optional
    }
  }
}
```

#### 7. **logdump_stats** - Get system statistics
```json
{
  "method": "tools/call",
  "params": {
    "name": "logdump_stats",
    "arguments": {}
  }
}
```

### Resources (MCP Resources)

You can also read logs as resources:

#### List Resources
```json
{
  "method": "resources/list",
  "id": 1
}
```

#### Read a Stream Resource
```json
{
  "method": "resources/read",
  "params": {
    "uri": "logdump://stream/app"
  },
  "id": 2
}
```

#### Read a Group Resource
```json
{
  "method": "resources/read",
  "params": {
    "uri": "logdump://group/errors"
  },
  "id": 3
}
```

### Example Workflow

```json
// 1. Set identity
{"method": "logdump/set_agent", "params": {"agent_id": "debug-001", "agent_name": "Claude"}, "id": 1}

// 2. Check available streams
{"method": "tools/call", "params": {"name": "logdump_streams", "arguments": {}}, "id": 2}

// 3. Search for errors in the last hour
{"method": "tools/call", "params": {"name": "logdump_grep", "arguments": {"pattern": "ERROR", "limit": 50}}, "id": 3}

// 4. Get a specific stream's logs
{"method": "tools/call", "params": {"name": "logdump_read", "arguments": {"source": "app", "limit": 20}}, "id": 4}

// 5. Create a group for database issues
{"method": "tools/call", "params": {"name": "logdump_create_group", "arguments": {"name": "db-issues", "pattern": "database|connection|timeout", "color": "red", "streams": "app"}}, "id": 5}

// 6. Read logs from a group resource
{"method": "resources/read", "params": {"uri": "logdump://group/db-issues"}, "id": 6}

// 7. View who accessed logs recently
{"method": "tools/call", "params": {"name": "logdump_access_log", "arguments": {"limit": 20}}, "id": 7}
```

### Important Notes

1. **Each line in a log file = one log entry** - Multi-line messages appear as separate entries
2. **Groups filter by regex pattern** - Use `|` for OR, `.*` for wildcards
3. **Agent actions are tracked** - All access is logged with timestamps
4. **Case-insensitive search**: Use `"case_insensitive": true` in grep
5. **Groups combine multiple streams** - Useful for viewing related logs together
6. **Resources provide log content** - Use `resources/read` to get formatted log output

### Configuration File Example

```yaml
streams:
  - name: app
    path: /var/log/app
    patterns: ["*.log"]
    color: cyan

  - name: nginx
    path: /var/log/nginx
    patterns: ["access.log", "error.log"]
    color: yellow

groups:
  - name: errors
    pattern: "ERROR|FATAL|ERR"
    color: red
    streams:
      - app
      - nginx

  - name: web
    pattern: ""
    streams:
      - nginx
```

### Troubleshooting

- **No streams visible?** - Check the log files exist and are readable
- **Empty results?** - Try increasing `limit` or check your pattern
- **Groups not working?** - Ensure streams are correctly named in config
- **Resources return empty?** - Wait a moment for logs to be read into buffer


### Setting Your Identity

Always set your agent identity first:

```json
{
  "method": "logdump/set_agent",
  "params": {
    "agent_id": "unique-agent-id",
    "agent_name": "Your Name"
  },
  "id": 1
}
```

### Available Tools

#### 1. **logdump_streams** - List all log streams
```json
{
  "method": "tools/call",
  "params": {
    "name": "logdump_streams",
    "arguments": {}
  }
}
```

#### 2. **logdump_groups** - List log groups
```json
{
  "method": "tools/call",
  "params": {
    "name": "logdump_groups",
    "arguments": {}
  }
}
```

#### 3. **logdump_read** - Read log entries
```json
{
  "method": "tools/call",
  "params": {
    "name": "logdump_read",
    "arguments": {
      "source": "app",        // optional: filter by stream name
      "group": "errors",      // optional: filter by group name
      "limit": 100            // optional: max entries (default 100)
    }
  }
}
```

#### 4. **logdump_grep** - Search logs with regex
```json
{
  "method": "tools/call",
  "params": {
    "name": "logdump_grep",
    "arguments": {
      "pattern": "ERROR.*connection",
      "source": "app",              // optional
      "group": "errors",            // optional
      "limit": 50,                  // optional
      "case_insensitive": true      // optional
    }
  }
}
```

#### 5. **logdump_create_group** - Create a new log group
```json
{
  "method": "tools/call",
  "params": {
    "name": "logdump_create_group",
    "arguments": {
      "name": "database-errors",
      "pattern": "ERROR.*database|connection.*failed",
      "color": "red",
      "streams": "app,api,database"
    }
  }
}
```

#### 6. **logdump_access_log** - View agent access history
```json
{
  "method": "tools/call",
  "params": {
    "name": "logdump_access_log",
    "arguments": {
      "agent": "Claude",      // optional: filter by agent
      "limit": 50             // optional
    }
  }
}
```

#### 7. **logdump_stats** - Get system statistics
```json
{
  "method": "tools/call",
  "params": {
    "name": "logdump_stats",
    "arguments": {}
  }
}
```

### Example Workflow

```json
// 1. Set identity
{"method": "logdump/set_agent", "params": {"agent_id": "debug-001", "agent_name": "Claude"}, "id": 1}

// 2. Check available streams
{"method": "tools/call", "params": {"name": "logdump_streams", "arguments": {}}, "id": 2}

// 3. Search for errors in the last hour
{"method": "tools/call", "params": {"name": "logdump_grep", "arguments": {"pattern": "ERROR", "limit": 50}}, "id": 3}

// 4. Get a specific stream's logs
{"method": "tools/call", "params": {"name": "logdump_read", "arguments": {"source": "app", "limit": 20}}, "id": 4}

// 5. Create a group for database issues
{"method": "tools/call", "params": {"name": "logdump_create_group", "arguments": {"name": "db-issues", "pattern": "database|connection|timeout", "color": "red", "streams": "app"}}, "id": 5}

// 6. View who accessed logs recently
{"method": "tools/call", "params": {"name": "logdump_access_log", "arguments": {"limit": 20}}, "id": 6}
```

### Important Notes

1. **Each line in a log file = one log entry** - Multi-line messages appear as separate entries
2. **Groups filter by regex pattern** - Use `|` for OR, `.*` for wildcards
3. **Agent actions are tracked** - All access is logged with timestamps
4. **Case-insensitive search**: Use `"case_insensitive": true` in grep
5. **Groups combine multiple streams** - Useful for viewing related logs together

### Configuration File Example

```yaml
streams:
  - name: app
    path: /var/log/app
    patterns: ["*.log"]
    color: cyan

  - name: nginx
    path: /var/log/nginx
    patterns: ["access.log", "error.log"]
    color: yellow

groups:
  - name: errors
    pattern: "ERROR|FATAL|ERR"
    color: red
    streams:
      - app
      - nginx

  - name: web
    pattern: ""
    streams:
      - nginx
```

### Troubleshooting

- **No streams visible?** - Check the log files exist and are readable
- **Empty results?** - Try increasing `limit` or check your pattern
- **Groups not working?** - Ensure streams are correctly named in config
