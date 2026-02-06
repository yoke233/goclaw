package input

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// readModel 输入模型
type readModel struct {
	textInput textinput.Model
	quitting  bool
}

func (m readModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m readModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			return m, tea.Quit
		case tea.KeyEnter:
			return m, tea.Quit
		}
	}

	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m readModel) View() string {
	if m.quitting {
		return ""
	}
	return m.textInput.View()
}

// ReadLine 读取一行输入（支持中文）
func ReadLine(prompt string) (string, error) {
	ti := textinput.New()
	ti.Prompt = prompt
	ti.Placeholder = ""
	ti.Focus()

	m := readModel{textInput: ti}

	p := tea.NewProgram(m, tea.WithOutput(os.Stderr))
	finalModel, err := p.Run()
	if err != nil {
		return "", err
	}

	rm, ok := finalModel.(readModel)
	if !ok {
		return "", nil
	}
	if rm.quitting {
		return "", fmt.Errorf("interrupted")
	}

	return rm.textInput.Value(), nil
}
