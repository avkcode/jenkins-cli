package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/avkcode/jenkins-cli/pkg/client"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	dashboardRefresh time.Duration

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	sectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF"))

	itemStyle = lipgloss.NewStyle().
			PaddingLeft(2)

	greenStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00"))
	redStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555"))
	yellowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF55"))
	blueStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#5555FF"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
)

type jobInfo struct {
	Name   string
	Color  string
	Active bool
}

type nodeInfo struct {
	Name      string
	Online    bool
	Idle      bool
	Executors int
}

type model struct {
	client     *client.JenkinsClient
	jobs       []jobInfo
	nodes      []nodeInfo
	queueDepth int
	err        string
	quitting   bool
}

type tickMsg time.Time

func (m model) Init() tea.Cmd {
	return tick()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "r":
			return m, tick()
		}
	case tickMsg:
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		jobs, err := m.client.Client.GetAllJobs(ctx)
		if err != nil {
			m.err = err.Error()
		} else {
			m.err = ""
			m.jobs = m.jobs[:0]
			for _, j := range jobs {
				m.jobs = append(m.jobs, jobInfo{
					Name:   j.Raw.Name,
					Color:  j.Raw.Color,
					Active: j.Raw.Color == "red" || j.Raw.Color == "yellow" || j.Raw.Color == "red_anime" || j.Raw.Color == "yellow_anime",
				})
			}
		}

		nodes, err := m.client.Client.GetAllNodes(ctx)
		if err == nil {
			m.nodes = m.nodes[:0]
			for _, n := range nodes {
				name := n.GetName()
				if name == "" {
					name = "(master)"
				}
				m.nodes = append(m.nodes, nodeInfo{
					Name:      name,
					Online:    !n.Raw.Offline,
					Idle:      n.Raw.Idle,
					Executors: len(n.Raw.Executors),
				})
			}
		}

		queue, err := m.client.Client.GetQueue(ctx)
		if err == nil {
			m.queueDepth = len(queue.Raw.Items)
		}

		return m, tick()
	}
	return m, nil
}

func tick() tea.Cmd {
	return tea.Tick(dashboardRefresh, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) View() string {
	if m.quitting {
		return "Bye!\n"
	}

	s := titleStyle.Render("Jenkins Firecracker Dashboard") + "\n\n"

	if m.err != "" {
		s += redStyle.Render("Error: "+m.err) + "\n\n"
	}

	// Stats summary
	activeCount := 0
	onlineCount := 0
	for _, j := range m.jobs {
		if j.Active {
			activeCount++
		}
	}
	for _, n := range m.nodes {
		if n.Online {
			onlineCount++
		}
	}

	s += dimStyle.Render(fmt.Sprintf("Jobs: %d total (%d active) | Nodes: %d online | Queue: %d\n\n",
		len(m.jobs), activeCount, onlineCount, m.queueDepth))

	// Active jobs
	s += sectionStyle.Render("Active Jobs") + "\n"
	hasActive := false
	for _, j := range m.jobs {
		if !j.Active {
			continue
		}
		hasActive = true
		color := colorForStatus(j.Color)
		s += itemStyle.Render(fmt.Sprintf("● %s %s", j.Name, color.Render("("+j.Color+")"))) + "\n"
	}
	if !hasActive {
		s += itemStyle.Render(dimStyle.Render("No active builds")) + "\n"
	}

	// Queue
	if m.queueDepth > 0 {
		s += "\n" + sectionStyle.Render(fmt.Sprintf("Build Queue (%d items)", m.queueDepth)) + "\n"
	}

	// Nodes
	s += "\n" + sectionStyle.Render("Nodes") + "\n"
	for _, n := range m.nodes {
		status := greenStyle.Render("● Online")
		if !n.Online {
			status = redStyle.Render("● Offline")
		}
		idle := ""
		if !n.Idle {
			idle = yellowStyle.Render(" [busy]")
		}
		s += itemStyle.Render(fmt.Sprintf("%s %s (%d exec)%s\n", status, n.Name, n.Executors, idle))
	}

	s += "\n" + dimStyle.Render("q: quit  r: refresh") + "\n"
	return s
}

func colorForStatus(color string) lipgloss.Style {
	switch color {
	case "red", "red_anime":
		return redStyle
	case "yellow", "yellow_anime":
		return yellowStyle
	case "blue", "blue_anime":
		return blueStyle
	case "green":
		return greenStyle
	default:
		return dimStyle
	}
}

var dashboardCmd = &cobra.Command{
	Use:     "dashboard",
	Short:   "Interactive TUI dashboard",
	GroupID: GroupCore,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := getClient(ctx)
		if err != nil {
			return err
		}

		p := tea.NewProgram(model{client: client})
		if _, err := p.Run(); err != nil {
			return err
		}
		return nil
	},
}

func init() {
	dashboardCmd.Flags().DurationVarP(&dashboardRefresh, "refresh", "r", 3*time.Second, "Dashboard refresh interval")
	rootCmd.AddCommand(dashboardCmd)
}
