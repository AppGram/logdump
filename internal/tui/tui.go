package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/appgram/logdump/internal/config"
	"github.com/appgram/logdump/internal/logtail"
)

var (
	headerBg    = lipgloss.NewStyle().Background(lipgloss.Color("#3d3d5c")).Foreground(lipgloss.Color("#ffffff"))
	headerCell  = lipgloss.NewStyle().Width(13).Background(lipgloss.Color("#3d3d5c")).Foreground(lipgloss.Color("#ffffff")).Bold(true).Align(lipgloss.Center)
	borderStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#4d4d6a"))
	helpBar     = lipgloss.NewStyle().Background(lipgloss.Color("#2d2d44")).Foreground(lipgloss.Color("#888888")).Padding(0, 1)
	titleStyle  = lipgloss.NewStyle().Background(lipgloss.Color("#00d9ff")).Foreground(lipgloss.Color("#1a1a2e")).Bold(true).Padding(0, 1)

	errorColor   = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555"))
	cyanColor    = lipgloss.NewStyle().Foreground(lipgloss.Color("#00d9ff"))
	greenColor   = lipgloss.NewStyle().Foreground(lipgloss.Color("#55ff55"))
	yellowColor  = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffaa00"))
	magentaColor = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff55ff"))
	blueColor    = lipgloss.NewStyle().Foreground(lipgloss.Color("#55aaff"))
	whiteColor   = lipgloss.NewStyle().Foreground(lipgloss.Color("#e0e0e0"))
	grayColor    = lipgloss.NewStyle().Foreground(lipgloss.Color("#666688"))

	cornerTL = "╭"
	cornerTR = "╮"
	cornerBL = "╰"
	cornerBR = "╯"
	horiz    = "─"
	vert     = "│"
	teeUp    = "┬"
	teeBoth  = "┼"
)

type LogEntry struct {
	Timestamp  string
	Source     string
	Content    string
	Tags       []string
	LineNumber int
}

type Model struct {
	manager         *logtail.Manager
	config          *config.Config
	viewport        viewport.Model
	logBuffer       []LogEntry
	filteredBuffer  []LogEntry
	searchQuery     string
	searchMode      bool
	streams         []string
	selectedStreams map[string]bool
	width           int
	height          int
	scrollOffset    int
	paused          bool
	autoScroll      bool
	selectedIdx     int
	detailMode      bool
	reverseOrder    bool
	showStreamList  bool
	confirmDelete   bool
	splashScreen    bool
	asciiArt        string
}

func New(manager *logtail.Manager, cfg *config.Config) *Model {
	vp := viewport.New(80, 20)
	vp.Style = lipgloss.NewStyle()

	streams := make([]string, 0, len(cfg.Streams))
	selectedStreams := make(map[string]bool)
	for _, s := range cfg.Streams {
		streams = append(streams, s.Name)
		selectedStreams[s.Name] = true
	}

	asciiArt := loadASCIIArt()

	return &Model{
		manager:         manager,
		config:          cfg,
		viewport:        vp,
		logBuffer:       make([]LogEntry, 0, 1000),
		filteredBuffer:  make([]LogEntry, 0, 1000),
		streams:         streams,
		selectedStreams: selectedStreams,
		autoScroll:      true,
		splashScreen:    true,
		asciiArt:        asciiArt,
	}
}

func loadASCIIArt() string {
	data, err := os.ReadFile("logdump-ascii.txt")
	if err != nil {
		return ""
	}
	return string(data)
}

func (m *Model) Init() tea.Cmd {
	if m.splashScreen {
		return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
			return splashTimeoutMsg(t)
		})
	}
	return m.tick()
}

type splashTimeoutMsg time.Time

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width - 4
		m.viewport.Height = msg.Height - 8
		m.viewport.SetContent(m.renderTable())

	case splashTimeoutMsg:
		m.splashScreen = false
		m.viewport.SetContent(m.renderTable())
		return m, m.tick()

	case tea.KeyMsg:
		// Handle search mode input FIRST - capture all typeable characters
		if m.searchMode {
			switch msg.String() {
			case "esc":
				m.searchMode = false
				m.searchQuery = ""
				m.filteredBuffer = m.logBuffer
				m.viewport.SetContent(m.renderTable())
			case "enter":
				m.searchMode = false
				m.applySearch(m.searchQuery)
			case "backspace":
				if len(m.searchQuery) > 0 {
					m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
				}
				m.applySearch(m.searchQuery)
			default:
				if len(msg.Runes) > 0 {
					m.searchQuery += string(msg.Runes)
				}
				m.applySearch(m.searchQuery)
			}
			return m, nil
		}

		// Normal mode key handling
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "?":
			m.viewport.SetContent(m.renderTable())

		case "/":
			m.searchMode = true

		case "esc":
			if m.confirmDelete {
				m.confirmDelete = false
			} else if m.detailMode {
				m.detailMode = false
				m.viewport.SetContent(m.renderTable())
			} else if m.showStreamList {
				m.showStreamList = false
			}

		case "enter":
			if m.confirmDelete {
				m.deleteLogFiles()
				m.confirmDelete = false
				m.logBuffer = make([]LogEntry, 0, 1000)
				m.filteredBuffer = m.logBuffer
				m.scrollOffset = 0
				m.viewport.SetContent(m.renderTable())
			} else if len(m.filteredBuffer) > 0 && m.selectedIdx < len(m.filteredBuffer) {
				m.detailMode = !m.detailMode
			}

		case "D":
			m.confirmDelete = true

		case "up", "k":
			if m.selectedIdx > 0 {
				m.selectedIdx--
				// Scroll up if selection goes above visible area
				if m.selectedIdx < m.scrollOffset {
					m.scrollOffset = m.selectedIdx
				}
				m.autoScroll = false
				m.viewport.SetContent(m.renderTable())
			}

		case "down", "j":
			if m.selectedIdx < len(m.filteredBuffer)-1 {
				m.selectedIdx++
				// Scroll down if selection goes below visible area
				visibleEnd := m.scrollOffset + m.viewport.Height - 1
				if m.selectedIdx > visibleEnd {
					m.scrollOffset = m.selectedIdx - m.viewport.Height + 1
				}
				// re-enable auto-scroll if at bottom
				if m.selectedIdx >= len(m.filteredBuffer)-1 {
					m.autoScroll = true
				}
				m.viewport.SetContent(m.renderTable())
			}

		case "pgup", "ctrl+u":
			m.scrollOffset = max(0, m.scrollOffset-m.viewport.Height)
			m.autoScroll = false // disable auto-scroll when scrolling up
			m.viewport.SetContent(m.renderTable())

		case "pgdn", "ctrl+d":
			maxScroll := max(0, len(m.filteredBuffer)-m.viewport.Height)
			m.scrollOffset = min(m.scrollOffset+m.viewport.Height, maxScroll)
			// re-enable auto-scroll if at bottom
			if m.scrollOffset >= maxScroll {
				m.autoScroll = true
			}
			m.viewport.SetContent(m.renderTable())

		case "home", "g":
			m.scrollOffset = 0
			m.autoScroll = false // disable auto-scroll when going to top
			m.viewport.SetContent(m.renderTable())

		case "end", "G":
			m.scrollOffset = max(0, len(m.filteredBuffer)-m.viewport.Height)
			m.autoScroll = true // re-enable auto-scroll when going to bottom
			m.viewport.SetContent(m.renderTable())

		case "c":
			m.logBuffer = make([]LogEntry, 0, 1000)
			m.filteredBuffer = m.logBuffer
			m.scrollOffset = 0
			m.viewport.SetContent(m.renderTable())

		case "p", " ":
			m.paused = !m.paused

		case "r":
			m.reverseOrder = !m.reverseOrder
			m.scrollOffset = 0
			m.selectedIdx = 0
			m.viewport.SetContent(m.renderTable())

		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			idx := int(msg.Runes[0] - '1')
			if idx >= 0 && idx < len(m.streams) {
				stream := m.streams[idx]
				m.selectedStreams[stream] = !m.selectedStreams[stream]
				m.applyFilters()
				m.viewport.SetContent(m.renderTable())
			}

		case "a":
			for _, s := range m.streams {
				m.selectedStreams[s] = true
			}
			m.applyFilters()
			m.viewport.SetContent(m.renderTable())

		case "n":
			for _, s := range m.streams {
				m.selectedStreams[s] = false
			}
			m.applyFilters()
			m.viewport.SetContent(m.renderTable())

		case "s":
			m.showStreamList = !m.showStreamList
		}

	case tickMsg:
		if !m.paused {
			m.updateLogs()
		}
		return m, m.tick()
	}

	return m, nil
}

func (m *Model) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	if m.splashScreen {
		return m.renderSplashScreen()
	}

	if m.confirmDelete {
		return m.renderDeleteConfirm()
	}

	if m.detailMode && len(m.filteredBuffer) > 0 && m.selectedIdx < len(m.filteredBuffer) {
		return m.renderDetailView()
	}

	if m.showStreamList {
		return m.renderStreamList()
	}

	table := m.renderTable()
	footer := m.renderFooter()

	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderTitleBar(),
		borderStyle.Render(table),
		footer,
	)
}

func (m *Model) renderSplashScreen() string {
	lines := strings.Split(m.asciiArt, "\n")

	var maxWidth int
	for _, line := range lines {
		if len(line) > maxWidth {
			maxWidth = len(line)
		}
	}

	paddingTop := (m.height - len(lines)) / 2
	sidePadding := (m.width - maxWidth) / 2

	var content strings.Builder
	for i := 0; i < paddingTop; i++ {
		content.WriteString("\n")
	}

	for _, line := range lines {
		spaces := sidePadding
		if spaces < 0 {
			spaces = 0
		}
		content.WriteString(strings.Repeat(" ", spaces))
		content.WriteString(cyanColor.Render(line))
		content.WriteString("\n")
	}

	helpMsg := grayColor.Render("Press any key to continue...")
	helpPadding := (m.width - lipgloss.Width(helpMsg)) / 2
	if helpPadding < 0 {
		helpPadding = 0
	}

	content.WriteString(strings.Repeat("\n", paddingTop-2))
	content.WriteString(strings.Repeat(" ", helpPadding))
	content.WriteString(helpMsg)
	content.WriteString("\n")

	return lipgloss.NewStyle().Height(m.height).Width(m.width).Render(content.String())
}

func (m *Model) renderStreamList() string {
	title := titleStyle.Render(" STREAMS ")
	header := headerBg.Width(m.width).Render(title + strings.Repeat(" ", max(0, m.width-lipgloss.Width(title))))

	var content strings.Builder
	content.WriteString("\n")
	content.WriteString(cyanColor.Render("  Press number key to toggle stream on/off:\n\n"))

	for i, s := range m.streams {
		var indicator string
		var status string
		if m.selectedStreams[s] {
			indicator = m.sourceColor(s).Render("●")
			status = greenColor.Render("ON ")
		} else {
			indicator = grayColor.Render("○")
			status = grayColor.Render("OFF")
		}

		keyNum := i + 1
		keyStyle := cyanColor.Bold(true)
		if keyNum > 9 {
			keyStyle = grayColor // Can't toggle with single key
		}

		line := fmt.Sprintf("  %s  %s %s  %s\n",
			keyStyle.Render(fmt.Sprintf("[%d]", keyNum)),
			indicator,
			status,
			m.sourceColor(s).Render(s))
		content.WriteString(line)
	}

	content.WriteString("\n")
	content.WriteString(grayColor.Render("  [a] Select all  [n] Select none  [ESC/s] Close\n"))

	listBox := lipgloss.NewStyle().
		Width(m.width - 4).
		Height(m.height - 6).
		Render(content.String())

	footer := helpBar.Render(grayColor.Render(fmt.Sprintf("Total: %d streams", len(m.streams))))

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		borderStyle.Render(listBox),
		footer,
	)
}

func (m *Model) renderDeleteConfirm() string {
	title := titleStyle.Render(" DELETE LOGS ")
	header := headerBg.Width(m.width).Render(title + strings.Repeat(" ", max(0, m.width-lipgloss.Width(title))))

	var content strings.Builder
	content.WriteString("\n\n")
	content.WriteString(errorColor.Render("  ⚠ WARNING: This will permanently delete log file contents!\n\n"))
	content.WriteString(cyanColor.Render("  The following log files will be cleared:\n\n"))

	for _, stream := range m.config.Streams {
		if m.selectedStreams[stream.Name] {
			content.WriteString(fmt.Sprintf("    • %s (%s)\n", stream.Name, stream.Path))
		}
	}

	content.WriteString("\n")
	content.WriteString(whiteColor.Render("  Press ENTER to confirm, ESC to cancel\n"))

	confirmBox := lipgloss.NewStyle().
		Width(m.width - 4).
		Height(m.height - 6).
		Render(content.String())

	footer := helpBar.Render(errorColor.Render("[ENTER] Delete  ") + grayColor.Render("[ESC] Cancel"))

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		borderStyle.Render(confirmBox),
		footer,
	)
}

func (m *Model) renderDetailView() string {
	entry := m.filteredBuffer[m.selectedIdx]

	title := titleStyle.Render(" LOG DETAIL ")
	header := headerBg.Width(m.width).Render(title + strings.Repeat(" ", max(0, m.width-lipgloss.Width(title))))

	// Build detail content
	var content strings.Builder
	content.WriteString("\n")
	content.WriteString(cyanColor.Render("  Source:     ") + m.sourceColor(entry.Source).Render(entry.Source) + "\n")
	content.WriteString(cyanColor.Render("  Timestamp:  ") + whiteColor.Render(entry.Timestamp) + "\n")
	content.WriteString(cyanColor.Render("  Line:       ") + whiteColor.Render(fmt.Sprintf("%d", entry.LineNumber)) + "\n")
	if len(entry.Tags) > 0 {
		content.WriteString(cyanColor.Render("  Tags:       ") + whiteColor.Render(strings.Join(entry.Tags, ", ")) + "\n")
	}
	content.WriteString("\n")
	content.WriteString(cyanColor.Render("  Content:\n"))
	content.WriteString(grayColor.Render("  " + strings.Repeat("─", m.width-6) + "\n"))

	// Word wrap content for display, use stream color
	contentLines := m.wrapText(entry.Content, m.width-6)
	for _, line := range contentLines {
		content.WriteString("  " + m.sourceColor(entry.Source).Render(line) + "\n")
	}

	content.WriteString(grayColor.Render("  " + strings.Repeat("─", m.width-6) + "\n"))

	detailBox := lipgloss.NewStyle().
		Width(m.width - 4).
		Height(m.height - 6).
		Render(content.String())

	footer := helpBar.Render(grayColor.Render("[ESC/Enter] Back to list  [↑/↓] Navigate"))

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		borderStyle.Render(detailBox),
		footer,
	)
}

func (m *Model) wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	var lines []string
	for len(text) > width {
		// Find a good break point
		breakAt := width
		for i := width; i > 0; i-- {
			if text[i] == ' ' || text[i] == '-' || text[i] == ',' {
				breakAt = i + 1
				break
			}
		}
		lines = append(lines, text[:breakAt])
		text = text[breakAt:]
	}
	if len(text) > 0 {
		lines = append(lines, text)
	}
	return lines
}

func (m *Model) renderTitleBar() string {
	timeStr := time.Now().Format("15:04:05")
	title := titleStyle.Render(" LOGDUMP ")
	right := helpBar.Render(timeStr)

	// Calculate available width for stream indicators
	availableWidth := m.width - lipgloss.Width(title) - lipgloss.Width(right) - 6 // 6 for padding

	streamIndicators := make([]string, 0, len(m.streams))
	currentWidth := 0
	hiddenCount := 0

	for i, s := range m.streams {
		// Truncate long stream names
		displayName := s
		if len(displayName) > 12 {
			displayName = displayName[:10] + ".."
		}

		var indicator string
		if m.selectedStreams[s] {
			style := m.sourceColor(s).Bold(true)
			indicator = style.Render(fmt.Sprintf("[%d]● %s", i+1, displayName))
		} else {
			indicator = grayColor.Render(fmt.Sprintf("[%d]○ %s", i+1, displayName))
		}

		indicatorWidth := lipgloss.Width(indicator) + 2 // +2 for spacing

		// Check if adding this indicator would overflow
		if currentWidth+indicatorWidth > availableWidth && len(streamIndicators) > 0 {
			hiddenCount = len(m.streams) - i
			break
		}

		streamIndicators = append(streamIndicators, indicator)
		currentWidth += indicatorWidth
	}

	// Add "+N more" indicator if streams were hidden
	var streamsStr string
	if hiddenCount > 0 {
		streamsStr = strings.Join(streamIndicators, "  ") + grayColor.Render(fmt.Sprintf("  +%d more", hiddenCount))
	} else {
		streamsStr = strings.Join(streamIndicators, "  ")
	}

	left := title + "  " + streamsStr
	padding := max(0, m.width-lipgloss.Width(left)-lipgloss.Width(right))

	return headerBg.Width(m.width).Render(left + strings.Repeat(" ", padding) + right)
}

func (m *Model) renderTable() string {
	if len(m.filteredBuffer) == 0 {
		emptyMsg := cyanColor.Render("  No logs to display  ")
		helpMsg := grayColor.Render("  Press '?' for help  ")
		padding := m.width - lipgloss.Width(emptyMsg) - lipgloss.Width(helpMsg)
		if padding < 0 {
			padding = 0
		}
		return lipgloss.NewStyle().Height(m.viewport.Height).Width(m.viewport.Width).Render(
			"\n\n\n" + emptyMsg + strings.Repeat(" ", padding/2) + helpMsg + "\n\n",
		)
	}

	header := m.renderTableHeader()

	visibleRows := m.viewport.Height
	startIdx := m.scrollOffset
	endIdx := min(startIdx+visibleRows, len(m.filteredBuffer))

	var rows []string
	for i := startIdx; i < endIdx; i++ {
		// When reverse order is enabled, display entries from end to start
		entryIdx := i
		if m.reverseOrder {
			entryIdx = len(m.filteredBuffer) - 1 - i
		}
		entry := m.filteredBuffer[entryIdx]
		isSelected := i == m.selectedIdx
		row := m.renderTableRow(entry, i%2 == 1, isSelected)
		rows = append(rows, row)
	}

	contentHeight := m.viewport.Height
	for len(rows) < contentHeight {
		rows = append(rows, lipgloss.NewStyle().Width(m.viewport.Width).Render(""))
	}

	return header + "\n" + strings.Join(rows, "\n")
}

func (m *Model) renderTableHeader() string {
	timestamp := headerCell.Render("TIMESTAMP")
	source := lipgloss.NewStyle().Width(16).Background(lipgloss.Color("#3d3d5c")).Foreground(lipgloss.Color("#ffffff")).Bold(true).Padding(0, 1).Render("SOURCE")
	content := lipgloss.NewStyle().Width(m.viewport.Width-13-16-4).Background(lipgloss.Color("#3d3d5c")).Foreground(lipgloss.Color("#ffffff")).Bold(true).Align(lipgloss.Left).Padding(0, 1).Render("LOG CONTENT")

	borderLine := cornerTL + strings.Repeat(horiz, 13) + teeUp + strings.Repeat(horiz, 16) + teeUp + strings.Repeat(horiz, m.viewport.Width-13-16-4) + cornerTR
	headerLine := vert + timestamp + vert + source + vert + content + vert
	separator := cornerBL + strings.Repeat(horiz, 13) + teeBoth + strings.Repeat(horiz, 16) + teeBoth + strings.Repeat(horiz, m.viewport.Width-13-16-4) + cornerBR

	return borderLine + "\n" + headerLine + "\n" + separator
}

func (m *Model) renderTableRow(entry LogEntry, alt bool, selected bool) string {
	timestamp := grayColor.Render(entry.Timestamp[:12])

	indicator := "●"
	if !m.selectedStreams[entry.Source] {
		indicator = "○"
	}

	// Selection indicator
	selectIndicator := " "
	if selected {
		selectIndicator = cyanColor.Render("▶")
	}

	source := m.sourceColor(entry.Source).Render(indicator + " " + entry.Source)

	maxContentLen := m.viewport.Width - 13 - 16 - 8
	if maxContentLen < 10 {
		maxContentLen = 10
	}

	content := entry.Content
	if len(content) > maxContentLen {
		content = content[:maxContentLen-3] + "..."
	}

	// Use stream color for log content
	styledContent := m.sourceColor(entry.Source).Render(content)

	tsStyle := lipgloss.NewStyle().Width(12)
	srcStyle := lipgloss.NewStyle().Width(16)
	ctStyle := lipgloss.NewStyle().Width(maxContentLen + 2)

	if selected {
		// Highlight selected row
		tsStyle = tsStyle.Background(lipgloss.Color("#3d5c5c"))
		srcStyle = srcStyle.Background(lipgloss.Color("#3d5c5c"))
		ctStyle = ctStyle.Background(lipgloss.Color("#3d5c5c"))
	} else if alt {
		tsStyle = tsStyle.Background(lipgloss.Color("#1e1e2e"))
		srcStyle = srcStyle.Background(lipgloss.Color("#1e1e2e"))
		ctStyle = ctStyle.Background(lipgloss.Color("#1e1e2e"))
	}

	return selectIndicator + vert + tsStyle.Render(timestamp) + vert + srcStyle.Render(source) + vert + ctStyle.Render(" "+styledContent+" ") + vert
}

func (m *Model) renderFooter() string {
	status := ""
	if m.paused {
		status = errorColor.Render("[PAUSED] ")
	}
	if m.autoScroll {
		status += cyanColor.Render("[AUTO] ")
	}
	if m.reverseOrder {
		status += yellowColor.Render("[↓NEW] ")
	} else {
		status += greenColor.Render("[NEW↓] ")
	}

	if m.searchMode {
		searchInput := cyanColor.Render("/") + whiteColor.Render(m.searchQuery) + cyanColor.Render("█")
		searchBar := helpBar.Render(status + searchInput + "  (ESC: cancel, Enter: search)")
		return searchBar
	}

	stats := fmt.Sprintf("Lines: %d | Visible: %d/%d | Scroll: %d",
		len(m.logBuffer), len(m.filteredBuffer), 1000, m.scrollOffset)

	controls := grayColor.Render("[↑/↓]Select [Enter]Detail [/]Search [s]Streams [r]Reverse [c]Clear [D]Delete [p]Pause [q]Quit")

	helpBar2 := helpBar.Render(status + controls)
	return helpBar2 + "\n" + helpBar.Render(stats)
}

func (m *Model) sourceColor(source string) lipgloss.Style {
	for _, stream := range m.config.Streams {
		if stream.Name == source {
			switch strings.ToLower(stream.Color) {
			case "red":
				return errorColor
			case "green":
				return greenColor
			case "blue":
				return blueColor
			case "yellow":
				return yellowColor
			case "cyan":
				return cyanColor
			case "magenta":
				return magentaColor
			case "white":
				return whiteColor
			}
		}
	}
	return grayColor
}

func (m *Model) updateLogs() {
	select {
	case entry, ok := <-m.manager.Entries():
		if !ok {
			return
		}

		m.logBuffer = append(m.logBuffer, LogEntry{
			Timestamp:  entry.Timestamp.Format("15:04:05.000"),
			Source:     entry.Source,
			Content:    entry.Content,
			Tags:       entry.Tags,
			LineNumber: entry.LineNumber,
		})

		if len(m.logBuffer) > 1000 {
			m.logBuffer = m.logBuffer[len(m.logBuffer)-1000:]
		}

		if m.selectedStreams[entry.Source] {
			if m.searchQuery == "" || strings.Contains(
				strings.ToLower(entry.Content),
				strings.ToLower(m.searchQuery),
			) {
				m.filteredBuffer = append(m.filteredBuffer, LogEntry{
					Timestamp:  entry.Timestamp.Format("15:04:05.000"),
					Source:     entry.Source,
					Content:    entry.Content,
					Tags:       entry.Tags,
					LineNumber: entry.LineNumber,
				})

				if len(m.filteredBuffer) > 1000 {
					m.filteredBuffer = m.filteredBuffer[len(m.filteredBuffer)-1000:]
				}

				// Auto-scroll when new logs arrive
				if m.autoScroll {
					if m.reverseOrder {
						// In reverse order, newest is at top, so stay at top
						m.scrollOffset = 0
						m.selectedIdx = 0
					} else {
						// Normal order, newest at bottom, scroll to bottom
						m.scrollOffset = max(0, len(m.filteredBuffer)-m.viewport.Height)
						m.selectedIdx = len(m.filteredBuffer) - 1
					}
				}
			}
		}

		m.viewport.SetContent(m.renderTable())
	default:
	}
}

func (m *Model) applySearch(query string) {
	m.searchQuery = query

	if query == "" {
		m.applyFilters()
	} else {
		pattern := regexp.QuoteMeta(query)
		re := regexp.MustCompile("(?i)" + pattern)

		m.filteredBuffer = make([]LogEntry, 0)
		for _, entry := range m.logBuffer {
			if m.selectedStreams[entry.Source] && re.MatchString(entry.Content) {
				m.filteredBuffer = append(m.filteredBuffer, entry)
			}
		}
	}

	m.viewport.SetContent(m.renderTable())
}

func (m *Model) applyFilters() {
	m.filteredBuffer = make([]LogEntry, 0)
	for _, entry := range m.logBuffer {
		if m.selectedStreams[entry.Source] {
			m.filteredBuffer = append(m.filteredBuffer, entry)
		}
	}
}

func (m *Model) tick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

type tickMsg time.Time

func (m *Model) deleteLogFiles() {
	for _, stream := range m.config.Streams {
		if !m.selectedStreams[stream.Name] {
			continue
		}

		// Find log files matching the stream patterns
		for _, pattern := range stream.Patterns {
			matches, err := filepath.Glob(filepath.Join(stream.Path, pattern))
			if err != nil {
				continue
			}

			for _, match := range matches {
				// Truncate the file (clear contents but keep file)
				if err := os.Truncate(match, 0); err != nil {
					continue
				}
			}
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
