package input

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Model 输入模型
type Model struct {
	textInput   textinput.Model
	prompt      string
	submitted   bool
	quitting    bool
	history     []string
	historyIdx  int
	styles      styles
	allowEmpty  bool
}

type styles struct {
	prompt     lipgloss.Style
	cursor     lipgloss.Style
	placeholder lipgloss.Style
}

// Config 配置
type Config struct {
	Prompt     string
	Placeholder string
	CharLimit  int
	Width      int
	AllowEmpty bool
}

// New 创建输入模型
func New(cfg Config) Model {
	ti := textinput.New()
	ti.Placeholder = cfg.Placeholder
	ti.CharLimit = cfg.CharLimit
	ti.Width = cfg.Width
	ti.Focus()

	s := styles{
		prompt: lipgloss.NewStyle().Foreground(lipgloss.Color("212")),
		cursor: lipgloss.NewStyle().Foreground(lipgloss.Color("226")),
		placeholder: lipgloss.NewStyle().Faint(true),
	}

	return Model{
		textInput:  ti,
		prompt:     cfg.Prompt,
		styles:     s,
		allowEmpty: cfg.AllowEmpty,
		historyIdx: -1,
	}
}

// Init 初始化
func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

// Update 更新模型
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			return m, tea.Quit

		case tea.KeyEnter:
			if !m.allowEmpty && strings.TrimSpace(m.textInput.Value()) == "" {
				return m, nil
			}
			m.submitted = true
			// 保存到历史记录
			if m.textInput.Value() != "" {
				m.history = append(m.history, m.textInput.Value())
			}
			return m, tea.Quit

		case tea.KeyUp:
			// 浏览历史记录
			if len(m.history) > 0 {
				if m.historyIdx < len(m.history)-1 {
					m.historyIdx++
					m.textInput.SetValue(m.history[len(m.history)-1-m.historyIdx])
				}
			}

		case tea.KeyDown:
			// 浏览历史记录
			if m.historyIdx >= 0 {
				m.historyIdx--
				if m.historyIdx >= 0 {
					m.textInput.SetValue(m.history[len(m.history)-1-m.historyIdx])
				} else {
					m.textInput.SetValue("")
				}
			}
		}

		// 传递其他按键给 textinput
		m.textInput, cmd = m.textInput.Update(msg)
	}

	return m, cmd
}

// View 渲染视图
func (m Model) View() string {
	if m.quitting {
		return ""
	}
	return m.styles.prompt.Render(m.prompt) + " " + m.textInput.View()
}

// Value 获取输入值
func (m Model) Value() string {
	return m.textInput.Value()
}

// Submitted 是否已提交
func (m Model) Submitted() bool {
	return m.submitted
}

// Quitting 是否正在退出
func (m Model) Quitting() bool {
	return m.quitting
}

// Reset 重置输入
func (m Model) Reset() Model {
	m.submitted = false
	m.quitting = false
	m.textInput.SetValue("")
	m.textInput.Focus()
	m.historyIdx = -1
	return m
}

// Run 运行输入
func Run(cfg Config) (string, error) {
	model := New(cfg)
	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		return "", err
	}
	m, ok := finalModel.(tea.Model)
	if !ok {
		return "", nil
	}
	return m.(Model).Value(), nil
}

// Completer 补全器
type Completer interface {
	Complete(input string) []string
}

// CompleterModel 带补全的输入模型
type CompleterModel struct {
	Model
	completer   Completer
	suggestions []string
	showMenu    bool
	menuIndex   int
}

// NewWithCompleter 创建带补全的输入模型
func NewWithCompleter(cfg Config, completer Completer) tea.Model {
	return &CompleterModel{
		Model:    New(cfg),
		completer: completer,
	}
}

// Update 更新补全模型
func (m *CompleterModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyTab {
			// Tab 补全
			input := m.textInput.Value()
			m.suggestions = m.completer.Complete(input)
			if len(m.suggestions) > 0 {
				m.showMenu = true
				m.menuIndex = 0
			}
			return m, nil
		}

		if m.showMenu {
			switch msg.Type {
			case tea.KeyUp:
				if m.menuIndex > 0 {
					m.menuIndex--
				}
				return m, nil
			case tea.KeyDown:
				if m.menuIndex < len(m.suggestions)-1 {
					m.menuIndex++
				}
				return m, nil
			case tea.KeyEnter:
				// 选择建议
				if m.menuIndex < len(m.suggestions) {
					m.textInput.SetValue(m.suggestions[m.menuIndex])
				}
				m.showMenu = false
				m.suggestions = nil
				return m, nil
			case tea.KeyEsc:
				m.showMenu = false
				m.suggestions = nil
				return m, nil
			}
		}
	}

	// 传递给基础模型
	baseModel, baseCmd := m.Model.Update(msg)
	m.Model = baseModel.(Model)
	cmd = baseCmd

	return m, cmd
}

// View 渲染补全模型
func (m *CompleterModel) View() string {
	content := m.Model.View()
	if m.showMenu && len(m.suggestions) > 0 {
		menu := "\n"
		for i, s := range m.suggestions {
			prefix := "  "
			if i == m.menuIndex {
				prefix = "> "
			}
			menu += prefix + s + "\n"
		}
		content += m.styles.placeholder.Render(menu)
	}
	return content
}

// SimpleCompleter 简单补全器
type SimpleCompleter struct {
	options []string
}

func NewSimpleCompleter(options []string) *SimpleCompleter {
	sort.Strings(options)
	return &SimpleCompleter{options: options}
}

func (c *SimpleCompleter) Complete(input string) []string {
	var matches []string
	input = strings.ToLower(input)
	for _, opt := range c.options {
		if strings.HasPrefix(strings.ToLower(opt), input) {
			matches = append(matches, opt)
		}
	}
	return matches
}

// PrefixCompleter 前缀补全器
type PrefixCompleter struct {
	prefix  string
	options []string
}

func NewPrefixCompleter(prefix string, options []string) Completer {
	sort.Strings(options)
	return &PrefixCompleter{prefix: prefix, options: options}
}

func (c *PrefixCompleter) Complete(input string) []string {
	if !strings.HasPrefix(input, c.prefix) {
		return nil
	}
	remaining := strings.TrimPrefix(input, c.prefix)
	var matches []string
	for _, opt := range c.options {
		if strings.HasPrefix(strings.ToLower(opt), strings.ToLower(remaining)) {
			matches = append(matches, c.prefix+opt)
		}
	}
	return matches
}
