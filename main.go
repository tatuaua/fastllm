package main

import (
	"fmt"
	"log"
	"strings"

	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func main() {
	defer WriteLogs("fastllm.log")
	loadEnv()
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}

type errMsg struct{ error }

func (e errMsg) Error() string { return e.error.Error() }

// Styles
var (
	userLabel   = lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Bold(true)
	agentLabel  = lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true)
	userText    = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	agentText   = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	toolCmd     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	toolResult  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	hintStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	promptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true)
	spinStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

type model struct {
	textInput  textinput.Model
	viewport   viewport.Model
	spinner    spinner.Model
	response   string
	err        error
	messages   []string     // styled messages for display
	rawHistory string       // plain text history for LLM
	updates    chan tea.Msg // channel for intermediate agent updates
	working    bool         // true while agent loop is running
	quitting   bool
	pathMap    map[string]string
	width      int
	height     int
}

func initialModel() model {
	ti := textinput.New()
	ti.SetVirtualCursor(false)
	ti.Focus()
	ti.CharLimit = 256
	ti.SetWidth(30)
	ti.Prompt = promptStyle.Render("> ")

	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(10))
	vp.SetContent(hintStyle.Render("  Welcome to fastllm. Type a message to begin."))
	vp.KeyMap.Left.SetEnabled(false)
	vp.KeyMap.Right.SetEnabled(false)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = spinStyle

	return model{
		textInput: ti,
		viewport:  vp,
		spinner:   sp,
		pathMap:   BuildPathMap(),
		messages:  []string{},
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

const inputAreaHeight = 3

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.SetWidth(msg.Width)
		m.viewport.SetHeight(msg.Height - inputAreaHeight)
		m.textInput.SetWidth(msg.Width - 4)
		if len(m.messages) > 0 {
			m.refreshViewport()
		}
		m.viewport.GotoBottom()

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			if m.working {
				return m, nil
			}
			val := m.textInput.Value()
			if val == "" {
				return m, nil
			}
			m.rawHistory += "user: " + val + "\n"
			m.messages = append(m.messages, userLabel.Render("you: ")+userText.Render(val))
			m.refreshViewport()
			m.viewport.GotoBottom()
			m.working = true
			cmd := m.converseCmd()
			m.textInput.SetValue("")
			return m, tea.Batch(m.spinner.Tick, cmd)
		default:
			if m.working {
				return m, nil
			}
			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
			return m, cmd
		}

	case ToolCallMsg:
		cmd, _, _ := strings.Cut(msg.Command, " ")
		arg, _, _ := strings.Cut(strings.TrimPrefix(msg.Command, cmd+" "), "\n")
		if len(arg) > 60 {
			arg = arg[:60] + "..."
		}
		// Show only command name + first arg, and a short status line.
		status := msg.Result
		if idx := strings.IndexByte(status, '\n'); idx > 0 {
			status = status[:idx]
		}
		if len(status) > 80 {
			status = status[:80] + "..."
		}
		m.messages = append(m.messages,
			toolCmd.Render("  "+cmd+" ")+toolResult.Render(arg),
			toolResult.Render("    "+status),
		)
		m.refreshViewport()
		m.viewport.GotoBottom()
		return m, waitForUpdate(m.updates)

	case ResponseMsg:
		m.response = string(msg)
		m.rawHistory += "agent: " + m.response + "\n"
		m.messages = append(m.messages, agentLabel.Render("agent: ")+agentText.Render(m.response))
		m.refreshViewport()
		m.viewport.GotoBottom()
		return m, waitForUpdate(m.updates)

	case AgentDoneMsg:
		m.working = false
		return m, nil

	case errMsg:
		m.err = msg
		m.working = false
		m.messages = append(m.messages, errStyle.Render(fmt.Sprintf("error: %v", msg)))
		m.refreshViewport()
		m.viewport.GotoBottom()
		return m, nil

	case spinner.TickMsg:
		if !m.working {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case cursor.BlinkMsg:
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m *model) refreshViewport() {
	content := lipgloss.NewStyle().Width(m.viewport.Width()).Render(
		strings.Join(m.messages, "\n"),
	)
	// Bottom-align: pad the top so content sticks to the bottom.
	contentHeight := lipgloss.Height(content)
	if contentHeight < m.viewport.Height() {
		padding := strings.Repeat("\n", m.viewport.Height()-contentHeight)
		content = padding + content
	}
	m.viewport.SetContent(content)
}

func (m model) View() tea.View {
	vpView := m.viewport.View()

	var inputLine string
	if m.working {
		inputLine = "  " + m.spinner.View() + hintStyle.Render(" thinking...")
	} else {
		inputLine = m.textInput.View()
	}

	hint := hintStyle.Render("  esc quit · ↑↓ scroll")
	content := vpView + "\n" + inputLine + "\n" + hint

	v := tea.NewView(content)
	v.AltScreen = true

	if !m.working {
		c := m.textInput.Cursor()
		if c != nil {
			c.Y += lipgloss.Height(vpView)
		}
		v.Cursor = c
	}
	return v
}
