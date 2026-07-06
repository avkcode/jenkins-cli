package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type StepStatus int

const (
	StepPending StepStatus = iota
	StepRunning
	StepCompleted
	StepFailed
)

type Step struct {
	ID       int
	Label    string
	Status   StepStatus
	Duration time.Duration
	Info     string
	start    time.Time
}

type Model struct {
	Cloud    string
	Host     string
	Image    string
	CPU      float64
	RAM      float64
	Steps    []Step
	spinner  spinner.Model
	viewport viewport.Model
	logs     []string
	showLogs bool
	quitting bool
	err      error
	width    int
	height   int
	Action   chan string
}

func InitialModel(labels []string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	vp := viewport.New(0, 0)
	vp.Style = lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240"))

	steps := make([]Step, len(labels))
	for i, label := range labels {
		steps[i] = Step{
			ID:     i + 1,
			Label:  label,
			Status: StepPending,
		}
	}

	return Model{
		Steps:    steps,
		spinner:  s,
		viewport: vp,
		showLogs: true,
		Action:   make(chan string, 1),
	}
}

func (m Model) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width - 4
		m.viewport.Height = 10 // Fixed height for logs for now
		if m.height > 25 {
			m.viewport.Height = m.height - len(m.Steps) - 10
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "l":
			m.showLogs = !m.showLogs
		case "ctrl+x":
			return m, func() tea.Msg { return ActionMsg("kill") }
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case StepUpdateMsg:
		for i := range m.Steps {
			if m.Steps[i].ID == msg.StepID {
				m.Steps[i].Status = msg.Status
				if msg.Info != "" {
					m.Steps[i].Info = msg.Info
				}
				if msg.Status == StepRunning {
					m.Steps[i].start = time.Now()
				} else if msg.Status == StepCompleted || msg.Status == StepFailed {
					if !m.Steps[i].start.IsZero() {
						m.Steps[i].Duration = time.Since(m.Steps[i].start)
					}
				}
				break
			}
		}

		allDone := true
		for _, s := range m.Steps {
			if s.Status != StepCompleted && s.Status != StepFailed {
				allDone = false
				break
			}
		}
		if allDone {
			m.quitting = true
			return m, tea.Batch(tea.Quit)
		}

	case LogMsg:
		m.logs = append(m.logs, string(msg))
		if len(m.logs) > 500 {
			m.logs = m.logs[1:]
		}
		m.viewport.SetContent(strings.Join(m.logs, "\n"))
		if m.viewport.Height > 0 {
			m.viewport.GotoBottom()
		}

	case ConfigMsg:
		if msg.Cloud != "" {
			m.Cloud = msg.Cloud
		}
		if msg.Host != "" {
			m.Host = msg.Host
		}
		if msg.Image != "" {
			m.Image = msg.Image
		}

	case StatsMsg:
		m.CPU = msg.CPU
		m.RAM = msg.RAM

	case ActionMsg:
		select {
		case m.Action <- string(msg):
		default:
		}
		for i := range m.Steps {
			if m.Steps[i].Status == StepRunning {
				m.Steps[i].Info = fmt.Sprintf("Action: %s", string(msg))
			}
		}

	case error:
		m.err = msg
		m.quitting = true
		return m, tea.Quit
	}

	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	return m, tea.Batch(cmds...)
}

type StepUpdateMsg struct {
	StepID int
	Status StepStatus
	Info   string
}

type LogMsg string
type ActionMsg string
type StatsMsg struct {
	CPU float64
	RAM float64
}
type ConfigMsg struct {
	Cloud string
	Host  string
	Image string
}

func (m Model) View() string {
	if m.err != nil {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(fmt.Sprintf("\n  Error: %v\n", m.err))
	}

	var s strings.Builder

	// Header
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Background(lipgloss.Color("240")).
		Padding(0, 1)

	header := fmt.Sprintf(" Cloud: %s  |  Host: %s  |  Image: %s ",
		m.val(m.Cloud), m.val(m.Host), m.val(m.Image))
	s.WriteString("\n" + headerStyle.Render(header) + "\n")
	s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(strings.Repeat("-", lipgloss.Width(header))) + "\n")

	// Stats
	if m.CPU > 0 || m.RAM > 0 {
		cpuBar := m.progressBar(m.CPU / 100)
		ramBar := m.progressBar(m.RAM / 100)
		s.WriteString(fmt.Sprintf("  CPU: %s %3.0f%%  |  RAM: %s %3.0f%%\n", cpuBar, m.CPU, ramBar, m.RAM))
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(strings.Repeat("-", lipgloss.Width(header))) + "\n")
	}

	// Steps
	for _, step := range m.Steps {
		var icon string
		var labelStyle lipgloss.Style
		var durationStr string
		var infoStr string

		switch step.Status {
		case StepPending:
			icon = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("[ ]")
			labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		case StepRunning:
			icon = m.spinner.View()
			labelStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
		case StepCompleted:
			icon = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("[✔]")
			labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
			durationStr = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(fmt.Sprintf(" (%v)", step.Duration.Round(time.Millisecond)))
		case StepFailed:
			icon = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("[✘]")
			labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
			durationStr = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(fmt.Sprintf(" (%v)", step.Duration.Round(time.Millisecond)))
		}

		if step.Info != "" {
			infoStr = lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Render(fmt.Sprintf(" [%s]", step.Info))
		}

		s.WriteString(fmt.Sprintf("  %s %d. %s%s%s\n", icon, step.ID, labelStyle.Render(step.Label), infoStr, durationStr))
	}

	// Logs
	if m.showLogs {
		s.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render("  Live Logs (Firecracker Console):") + "\n")
		s.WriteString("  " + m.viewport.View() + "\n")
	}

	if m.quitting {
		s.WriteString("\n  Finished.\n")
	} else {
		footerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		s.WriteString("\n" + footerStyle.Render("  'q': quit | 'l': toggle logs | 'ctrl+x': kill build") + "\n")
	}

	return s.String()
}

func (m Model) val(s string) string {
	if s == "" {
		return "n/a"
	}
	return s
}

func (m Model) progressBar(percent float64) string {
	width := 10
	full := int(percent * float64(width))
	if full > width {
		full = width
	}
	if full < 0 {
		full = 0
	}
	empty := width - full

	barStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("35"))
	if percent > 0.8 {
		barStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("160"))
	} else if percent > 0.5 {
		barStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	}

	return "[" + barStyle.Render(strings.Repeat("█", full)) + strings.Repeat("░", empty) + "]"
}
