package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/appgram/logdump/internal/config"
	"github.com/appgram/logdump/internal/logtail"
)

type AgentAccess struct {
	AgentID     string    `json:"agent_id"`
	AgentName   string    `json:"agent_name"`
	Action      string    `json:"action"`
	Source      string    `json:"source"`
	Pattern     string    `json:"pattern,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
	ResultCount int       `json:"result_count"`
}

type LogGroup struct {
	Name      string    `json:"name"`
	Pattern   string    `json:"pattern"`
	Color     string    `json:"color"`
	Streams   []string  `json:"streams"`
	CreatedAt time.Time `json:"created_at"`
}

type Server struct {
	manager      *logtail.Manager
	config       *config.Config
	accessLog    []AgentAccess
	accessMu     sync.RWMutex
	logGroups    map[string]LogGroup
	groupsMu     sync.RWMutex
	currentAgent string
	logFile      *os.File
	logMu        sync.Mutex
}

type MCPRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      interface{}     `json:"id,omitempty"`
}

type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
	ID      interface{} `json:"id,omitempty"`
}

type MCPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

type Property struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Enum        []string `json:"enum,omitempty"`
}

func NewServer(manager *logtail.Manager, cfg *config.Config) *Server {
	groups := make(map[string]LogGroup)
	for _, g := range cfg.Groups {
		groups[g.Name] = LogGroup{
			Name:      g.Name,
			Pattern:   g.Pattern,
			Color:     g.Color,
			Streams:   g.Streams,
			CreatedAt: time.Now(),
		}
	}

	server := &Server{
		manager:   manager,
		config:    cfg,
		accessLog: make([]AgentAccess, 0, 1000),
		logGroups: groups,
	}

	// Open MCP activity log file
	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, ".local", "share", "logdump", "logs")
	_ = os.MkdirAll(logDir, 0755)

	logFile, err := os.OpenFile(
		filepath.Join(logDir, "mcp-activity.log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0644,
	)
	if err == nil {
		server.logFile = logFile
		server.logActivity("MCP server started")
	} else {
		log.Printf("Warning: Could not open MCP activity log: %v", err)
	}

	return server
}

func (s *Server) logActivity(message string) {
	if s.logFile == nil {
		return
	}

	s.logMu.Lock()
	defer s.logMu.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	agent := s.currentAgent
	if agent == "" {
		agent = "unknown"
	}

	line := fmt.Sprintf("[%s] [AGENT: %s] %s\n", timestamp, agent, message)
	_, _ = s.logFile.WriteString(line)
	_ = s.logFile.Sync()
}

func (s *Server) logToolCall(toolName string, args map[string]interface{}, resultCount int) {
	if s.logFile == nil {
		return
	}

	s.logMu.Lock()
	defer s.logMu.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	agent := s.currentAgent
	if agent == "" {
		agent = "unknown"
	}

	argsJSON, _ := json.Marshal(args)
	resultInfo := ""
	if resultCount >= 0 {
		resultInfo = fmt.Sprintf(" -> %d results", resultCount)
	}

	line := fmt.Sprintf("[%s] [AGENT: %s] TOOL: %s(args: %s)%s\n", timestamp, agent, toolName, string(argsJSON), resultInfo)
	_, _ = s.logFile.WriteString(line)
	_ = s.logFile.Sync()
}

func (s *Server) RunStdio(ctx context.Context) error {
	return s.handleStdio(ctx, os.Stdin, os.Stdout)
}

func (s *Server) handleStdio(ctx context.Context, in io.Reader, out io.Writer) error {
	decoder := json.NewDecoder(in)
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			var rawReq map[string]interface{}
			if err := decoder.Decode(&rawReq); err != nil {
				if err == io.EOF {
					return nil
				}
				log.Printf("Error decoding request: %v", err)
				continue
			}

			var req MCPRequest
			if data, err := json.Marshal(rawReq); err == nil {
				_ = json.Unmarshal(data, &req)
			}

			if req.JSONRPC == "" {
				req.JSONRPC = "2.0"
			}

			resp := s.handleRequest(ctx, req)
			resp.JSONRPC = "2.0"

			if err := encoder.Encode(resp); err != nil {
				if err == io.EOF {
					return nil
				}
				log.Printf("Error encoding response: %v", err)
			}

			if f, ok := out.(interface{ Flush() }); ok {
				f.Flush()
			}
		}
	}
}

func (s *Server) RunWebsocket(ctx context.Context, addr string) error {
	http.HandleFunc("/", s.handleWebSocket)
	server := &http.Server{Addr: addr}

	go func() {
		<-ctx.Done()
		server.Close()
	}()

	return server.ListenAndServe()
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Upgrade") != "websocket" {
		http.Error(w, "Expected WebSocket", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	for {
		var rawReq map[string]interface{}
		if err := conn.ReadJSON(&rawReq); err != nil {
			if err != io.EOF {
				log.Printf("Error reading request: %v", err)
			}
			return
		}

		var req MCPRequest
		if data, err := json.Marshal(rawReq); err == nil {
			_ = json.Unmarshal(data, &req)
		}

		if req.JSONRPC == "" {
			req.JSONRPC = "2.0"
		}

		resp := s.handleRequest(r.Context(), req)
		resp.JSONRPC = "2.0"

		if err := conn.WriteJSON(resp); err != nil {
			log.Printf("Error writing response: %v", err)
		}
	}
}

func (s *Server) handleRequest(ctx context.Context, req MCPRequest) MCPResponse {
	id := req.ID
	if id == nil {
		id = json.RawMessage("null")
	}

	// Log the request
	s.logActivity(fmt.Sprintf("REQUEST: %s (id: %v)", req.Method, id))

	switch req.Method {
	case "initialize":
		return s.handleInitialize(req, id)
	case "tools/list":
		return s.handleToolsList(req, id)
	case "tools/call":
		return s.handleToolCall(ctx, req, id)
	case "resources/list":
		return s.handleResourcesList(req, id)
	case "resources/read":
		return s.handleResourcesRead(ctx, req, id)
	case "logdump/set_agent":
		return s.handleSetAgent(ctx, req, id)
	case "logdump/access_log":
		return s.handleAccessLog(req, id)
	case "ping":
		return MCPResponse{Result: map[string]interface{}{"status": "pong"}, ID: id}
	default:
		return MCPResponse{
			Error: &MCPError{
				Code:    -32600,
				Message: fmt.Sprintf("Invalid Request: unknown method '%s'", req.Method),
			},
			ID: id,
		}
	}
}

func (s *Server) handleInitialize(req MCPRequest, id interface{}) MCPResponse {
	return MCPResponse{
		Result: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{
					"list": true,
					"call": true,
				},
				"resources": map[string]interface{}{
					"list":      true,
					"read":      true,
					"subscribe": false,
				},
			},
			"serverInfo": map[string]interface{}{
				"name":    "logdump",
				"version": "1.0.0",
			},
		},
		ID: id,
	}
}

func (s *Server) handleToolsList(req MCPRequest, id interface{}) MCPResponse {
	tools := []Tool{
		{
			Name:        "logdump_read",
			Description: "Read log entries from active streams",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"source": {
						Type:        "string",
						Description: "Filter by stream name (optional)",
					},
					"group": {
						Type:        "string",
						Description: "Filter by log group name (optional)",
					},
					"limit": {
						Type:        "integer",
						Description: "Maximum number of entries to return (default 100)",
					},
				},
			},
		},
		{
			Name:        "logdump_grep",
			Description: "Search through log entries with regex pattern",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"pattern": {
						Type:        "string",
						Description: "Regex pattern to search for",
					},
					"source": {
						Type:        "string",
						Description: "Filter by stream name (optional)",
					},
					"group": {
						Type:        "string",
						Description: "Filter by log group name (optional)",
					},
					"limit": {
						Type:        "integer",
						Description: "Maximum number of results (default 100)",
					},
					"case_insensitive": {
						Type:        "boolean",
						Description: "Case insensitive search (default false)",
					},
				},
				Required: []string{"pattern"},
			},
		},
		{
			Name:        "logdump_streams",
			Description: "List all active log streams",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]Property{},
			},
		},
		{
			Name:        "logdump_groups",
			Description: "List all log groups",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]Property{},
			},
		},
		{
			Name:        "logdump_create_group",
			Description: "Create a new log group",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"name": {
						Type:        "string",
						Description: "Group name",
					},
					"pattern": {
						Type:        "string",
						Description: "Regex pattern to match logs",
					},
					"color": {
						Type:        "string",
						Description: "Color for display",
						Enum:        []string{"red", "green", "blue", "yellow", "cyan", "magenta"},
					},
					"streams": {
						Type:        "string",
						Description: "Comma-separated stream names",
					},
				},
				Required: []string{"name", "pattern"},
			},
		},
		{
			Name:        "logdump_stats",
			Description: "Get statistics about log streams and buffer",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]Property{},
			},
		},
		{
			Name:        "logdump_access_log",
			Description: "Get access log showing which agents accessed logs",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"agent": {
						Type:        "string",
						Description: "Filter by agent ID/name",
					},
					"limit": {
						Type:        "integer",
						Description: "Maximum entries to return (default 50)",
					},
				},
			},
		},
	}

	return MCPResponse{
		Result: map[string]interface{}{
			"tools": tools,
		},
		ID: id,
	}
}

func (s *Server) handleToolCall(ctx context.Context, req MCPRequest, id interface{}) MCPResponse {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		params.Arguments = make(map[string]interface{})
	}

	toolName := params.Name
	args := params.Arguments
	if args == nil {
		args = make(map[string]interface{})
	}

	agentID := s.currentAgent
	if agentID == "" {
		agentID = "unknown"
	}

	switch toolName {
	case "logdump_read":
		resp := s.toolRead(args, id, agentID)
		count := 0
		if r, ok := resp.Result.(map[string]interface{}); ok {
			if e, ok := r["count"].(float64); ok {
				count = int(e)
			}
		}
		s.logToolCall(toolName, args, count)
		return resp
	case "logdump_grep":
		resp := s.toolGrep(ctx, args, id, agentID)
		count := 0
		if r, ok := resp.Result.(map[string]interface{}); ok {
			if e, ok := r["count"].(float64); ok {
				count = int(e)
			}
		}
		s.logToolCall(toolName, args, count)
		return resp
	case "logdump_streams":
		resp := s.toolStreams(id, agentID)
		count := 0
		if r, ok := resp.Result.(map[string]interface{}); ok {
			if e, ok := r["count"].(float64); ok {
				count = int(e)
			}
		}
		s.logToolCall(toolName, args, count)
		return resp
	case "logdump_groups":
		resp := s.toolGroups(id, agentID)
		count := 0
		if r, ok := resp.Result.(map[string]interface{}); ok {
			if e, ok := r["count"].(float64); ok {
				count = int(e)
			}
		}
		s.logToolCall(toolName, args, count)
		return resp
	case "logdump_create_group":
		resp := s.toolCreateGroup(args, id, agentID)
		s.logToolCall(toolName, args, -1)
		return resp
	case "logdump_stats":
		resp := s.toolStats(id, agentID)
		s.logToolCall(toolName, args, -1)
		return resp
	case "logdump_access_log":
		resp := s.toolAccessLog(args, id, agentID)
		count := 0
		if r, ok := resp.Result.(map[string]interface{}); ok {
			if e, ok := r["count"].(float64); ok {
				count = int(e)
			}
		}
		s.logToolCall(toolName, args, count)
		return resp
	default:
		return MCPResponse{
			Error: &MCPError{
				Code:    -32601,
				Message: fmt.Sprintf("Method not found: %s", toolName),
			},
			ID: id,
		}
	}
}

func (s *Server) logAccess(agentID, action, source, pattern string, resultCount int) {
	s.accessMu.Lock()
	defer s.accessMu.Unlock()

	access := AgentAccess{
		AgentID:     agentID,
		Action:      action,
		Source:      source,
		Pattern:     pattern,
		Timestamp:   time.Now(),
		ResultCount: resultCount,
	}

	s.accessLog = append(s.accessLog, access)
	if len(s.accessLog) > 1000 {
		s.accessLog = s.accessLog[len(s.accessLog)-1000:]
	}
}

func (s *Server) toolRead(params map[string]interface{}, id interface{}, agentID string) MCPResponse {
	source, _ := params["source"].(string)
	group, _ := params["group"].(string)
	limit := 100
	if l, ok := params["limit"].(float64); ok {
		limit = int(l)
	}

	entries := s.manager.GetEntries(source, limit)

	var filtered []logtail.LogEntry
	if group != "" {
		s.groupsMu.RLock()
		g, ok := s.logGroups[group]
		s.groupsMu.RUnlock()
		if ok && g.Pattern != "" {
			re := regexp.MustCompile("(?i)" + g.Pattern)
			for _, e := range entries {
				if re.MatchString(e.Content) {
					filtered = append(filtered, e)
				}
			}
			entries = filtered
		}
	}

	var lines []string
	for _, entry := range entries {
		lines = append(lines, fmt.Sprintf("[%s] [%s] %s",
			entry.Timestamp.Format("15:04:05"),
			entry.Source,
			entry.Content))
	}

	text := strings.Join(lines, "\n")
	if len(entries) == 0 {
		text = "No log entries found"
	}

	s.logAccess(agentID, "read", source, "", len(entries))

	return MCPResponse{
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": text,
				},
			},
		},
		ID: id,
	}
}

func (s *Server) toolGrep(ctx context.Context, params map[string]interface{}, id interface{}, agentID string) MCPResponse {
	pattern, _ := params["pattern"].(string)
	source, _ := params["source"].(string)
	group, _ := params["group"].(string)
	limit := 100
	if l, ok := params["limit"].(float64); ok {
		limit = int(l)
	}
	caseInsensitive := false
	if ci, ok := params["case_insensitive"].(bool); ok {
		caseInsensitive = ci
	}

	flags := ""
	if caseInsensitive {
		flags = "(?i)"
	}

	fullPattern := flags + pattern

	var searchSource string
	if group != "" {
		s.groupsMu.RLock()
		g := s.logGroups[group]
		s.groupsMu.RUnlock()
		searchSource = strings.Join(g.Streams, ",")
	} else {
		searchSource = source
	}

	results, err := s.manager.Search(ctx, fullPattern, searchSource)
	if err != nil {
		return MCPResponse{
			Error: &MCPError{
				Code:    -32603,
				Message: err.Error(),
			},
			ID: id,
		}
	}

	var lines []string
	count := 0
	for entry := range results {
		if count >= limit {
			break
		}

		re, err := regexp.Compile(fullPattern)
		if err != nil {
			continue
		}

		if re.MatchString(entry.Content) {
			lines = append(lines, fmt.Sprintf("[%s] [%s] %s",
				entry.Timestamp.Format("15:04:05"),
				entry.Source,
				entry.Content))
			count++
		}
	}

	text := fmt.Sprintf("Pattern: %s\nMatches: %d\n\n%s", pattern, count, strings.Join(lines, "\n"))
	if count == 0 {
		text = fmt.Sprintf("Pattern: %s\nNo matches found", pattern)
	}

	s.logAccess(agentID, "grep", searchSource, pattern, count)

	return MCPResponse{
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": text,
				},
			},
		},
		ID: id,
	}
}

func (s *Server) toolStreams(id interface{}, agentID string) MCPResponse {
	streams := s.manager.GetStreams()

	var lines []string
	for path, stream := range streams {
		lines = append(lines, fmt.Sprintf("- %s: %s (%d lines read)",
			stream.Config.Name, path, stream.LineNumber))
	}

	text := fmt.Sprintf("Active Streams: %d\n\n%s", len(streams), strings.Join(lines, "\n"))
	if len(streams) == 0 {
		text = "No active streams"
	}

	s.logAccess(agentID, "list_streams", "", "", len(streams))

	return MCPResponse{
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": text,
				},
			},
		},
		ID: id,
	}
}

func (s *Server) toolGroups(id interface{}, agentID string) MCPResponse {
	s.groupsMu.RLock()
	defer s.groupsMu.RUnlock()

	var lines []string
	for _, group := range s.logGroups {
		lines = append(lines, fmt.Sprintf("- %s: pattern=%q streams=%v",
			group.Name, group.Pattern, group.Streams))
	}

	text := fmt.Sprintf("Log Groups: %d\n\n%s", len(s.logGroups), strings.Join(lines, "\n"))
	if len(s.logGroups) == 0 {
		text = "No log groups defined"
	}

	s.logAccess(agentID, "list_groups", "", "", len(s.logGroups))

	return MCPResponse{
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": text,
				},
			},
		},
		ID: id,
	}
}

func (s *Server) toolCreateGroup(params map[string]interface{}, id interface{}, agentID string) MCPResponse {
	name, _ := params["name"].(string)
	pattern, _ := params["pattern"].(string)
	color, _ := params["color"].(string)
	streamsStr, _ := params["streams"].(string)

	var streams []string
	if streamsStr != "" {
		parts := strings.Split(streamsStr, ",")
		for _, p := range parts {
			streams = append(streams, strings.TrimSpace(p))
		}
	}

	if color == "" {
		color = "cyan"
	}

	s.groupsMu.Lock()
	s.logGroups[name] = LogGroup{
		Name:      name,
		Pattern:   pattern,
		Color:     color,
		Streams:   streams,
		CreatedAt: time.Now(),
	}
	s.groupsMu.Unlock()

	s.logAccess(agentID, "create_group", name, pattern, 1)

	text := fmt.Sprintf("Created group '%s' with pattern '%s'", name, pattern)

	return MCPResponse{
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": text,
				},
			},
		},
		ID: id,
	}
}

func (s *Server) toolStats(id interface{}, agentID string) MCPResponse {
	streams := s.manager.GetStreams()
	streamCount := len(streams)

	s.groupsMu.RLock()
	groupCount := len(s.logGroups)
	s.groupsMu.RUnlock()

	bufferSize := len(s.manager.GetBuffer())

	s.logAccess(agentID, "stats", "", "", 0)

	text := fmt.Sprintf("Logdump Statistics:\n- Active streams: %d\n- Log groups: %d\n- Buffer size: %d entries\n- Access log: %d entries",
		streamCount, groupCount, bufferSize, len(s.accessLog))

	return MCPResponse{
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": text,
				},
			},
		},
		ID: id,
	}
}

func (s *Server) toolAccessLog(params map[string]interface{}, id interface{}, agentID string) MCPResponse {
	filterAgent, _ := params["agent"].(string)
	limit := 50
	if l, ok := params["limit"].(float64); ok {
		limit = int(l)
	}

	s.accessMu.RLock()
	defer s.accessMu.RUnlock()

	var filtered []AgentAccess
	if filterAgent != "" {
		for _, a := range s.accessLog {
			if a.AgentID == filterAgent || strings.Contains(strings.ToLower(a.AgentID), strings.ToLower(filterAgent)) {
				filtered = append(filtered, a)
			}
		}
	} else {
		filtered = s.accessLog
	}

	if limit > 0 && len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}

	var lines []string
	for _, a := range filtered {
		lines = append(lines, fmt.Sprintf("[%s] %s: %s (results: %d)",
			a.Timestamp.Format("15:04:05"), a.AgentID, a.Action, a.ResultCount))
	}

	text := fmt.Sprintf("Access Log: %d entries\n\n%s", len(filtered), strings.Join(lines, "\n"))
	if len(filtered) == 0 {
		text = "No access log entries"
	}

	return MCPResponse{
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": text,
				},
			},
		},
		ID: id,
	}
}

func (s *Server) handleSetAgent(ctx context.Context, req MCPRequest, id interface{}) MCPResponse {
	var params struct {
		AgentID   string `json:"agent_id"`
		AgentName string `json:"agent_name"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return MCPResponse{
			Error: &MCPError{
				Code:    -32602,
				Message: "Invalid params",
			},
			ID: id,
		}
	}

	s.currentAgent = fmt.Sprintf("%s (%s)", params.AgentName, params.AgentID)

	return MCPResponse{
		Result: map[string]interface{}{
			"success": true,
		},
		ID: id,
	}
}

func (s *Server) handleAccessLog(req MCPRequest, id interface{}) MCPResponse {
	return s.toolAccessLog(make(map[string]interface{}), id, "ui")
}

func (s *Server) handleResourcesList(req MCPRequest, id interface{}) MCPResponse {
	resources := make([]map[string]interface{}, 0)

	s.groupsMu.RLock()
	for name, group := range s.logGroups {
		resources = append(resources, map[string]interface{}{
			"uri":         fmt.Sprintf("logdump://group/%s", name),
			"name":        group.Name,
			"mimeType":    "text/plain",
			"description": fmt.Sprintf("Log group: %s (pattern: %s)", group.Name, group.Pattern),
		})
	}
	s.groupsMu.RUnlock()

	for _, stream := range s.config.Streams {
		resources = append(resources, map[string]interface{}{
			"uri":         fmt.Sprintf("logdump://stream/%s", strings.ToLower(stream.Name)),
			"name":        stream.Name,
			"mimeType":    "text/plain",
			"description": fmt.Sprintf("Log stream from %s", stream.Path),
		})
	}

	return MCPResponse{
		Result: map[string]interface{}{
			"resources": resources,
		},
		ID: id,
	}
}

func (s *Server) handleResourcesRead(ctx context.Context, req MCPRequest, id interface{}) MCPResponse {
	var params struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return MCPResponse{
			Error: &MCPError{
				Code:    -32602,
				Message: "Invalid params",
			},
			ID: id,
		}
	}

	uri := params.URI
	var text string

	if strings.HasPrefix(uri, "logdump://stream/") {
		streamName := strings.TrimPrefix(uri, "logdump://stream/")
		entries := s.manager.GetEntries(streamName, 100)
		var lines []string
		for _, e := range entries {
			lines = append(lines, fmt.Sprintf("[%s] %s | %s", e.Timestamp.Format("15:04:05.000"), e.Source, e.Content))
		}
		text = strings.Join(lines, "\n")
	} else if strings.HasPrefix(uri, "logdump://group/") {
		groupName := strings.TrimPrefix(uri, "logdump://group/")
		s.groupsMu.RLock()
		group, ok := s.logGroups[groupName]
		s.groupsMu.RUnlock()
		if ok {
			re := regexp.MustCompile("(?i)" + group.Pattern)
			entries := s.manager.GetEntries("", 100)
			var lines []string
			for _, e := range entries {
				for _, stream := range group.Streams {
					if e.Source == stream && re.MatchString(e.Content) {
						lines = append(lines, fmt.Sprintf("[%s] %s | %s", e.Timestamp.Format("15:04:05.000"), e.Source, e.Content))
						break
					}
				}
			}
			text = strings.Join(lines, "\n")
		} else {
			return MCPResponse{
				Error: &MCPError{
					Code:    -32603,
					Message: "Group not found: " + groupName,
				},
				ID: id,
			}
		}
	} else {
		return MCPResponse{
			Error: &MCPError{
				Code:    -32603,
				Message: "Unknown resource URI: " + uri,
			},
			ID: id,
		}
	}

	return MCPResponse{
		Result: map[string]interface{}{
			"contents": []map[string]interface{}{
				{
					"uri":      params.URI,
					"mimeType": "text/plain",
					"text":     text,
				},
			},
		},
		ID: id,
	}
}

var upgrader = &websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}
