package main

import (
	"log"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func main() {
	defer WriteLogs("fastllm.log")
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}

type errMsg struct{ error }

func (e errMsg) Error() string { return e.error.Error() }

type model struct {
	textInput textinput.Model
	response  string
	err       error
	history   string
	quitting  bool
	pathMap   map[string]string
}

func initialModel() model {
	ti := textinput.New()
	ti.SetVirtualCursor(false)
	ti.Focus()
	ti.CharLimit = 156
	ti.SetWidth(20)

	return model{
		textInput: ti,
		pathMap:   BuildPathMap(),
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			m.history += "user: " + m.textInput.Value() + "\n"
			cmd := m.converseCmd()
			m.textInput.SetValue("")
			return m, cmd
		}

	case ResponseMsg:
		m.response = string(msg)
		m.history += "agent: " + m.response + "\n"
		return m, nil

	case errMsg:
		m.err = msg
		return m, nil
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m model) View() tea.View {
	var c *tea.Cursor
	if !m.textInput.VirtualCursor() {
		c = m.textInput.Cursor()
		c.Y += lipgloss.Height(m.history)
	}

	styledHistory := lipgloss.NewStyle().Foreground(lipgloss.BrightBlack).Render(m.history)

	s := styledHistory
	s += "\n" + m.textInput.View() + "\n(esc to quit)"

	if m.quitting {
		s += "\n"
	}

	v := tea.NewView(s)
	v.Cursor = c
	return v
}
