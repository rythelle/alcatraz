package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/alcatraz/alcatraz/cli/internal/config"
	"github.com/alcatraz/alcatraz/cli/internal/docker"
	"github.com/alcatraz/alcatraz/cli/internal/workspace"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Screen represents the current TUI screen.
type Screen int

const (
	ScreenSplash Screen = iota
	ScreenDashboard
	ScreenRun
	ScreenExec
	ScreenWorkspaces
	ScreenStatus
	ScreenLogs
	ScreenTests
	ScreenConfirm
	ScreenOutput
)

// Msg types
type (
	TickMsg        time.Time
	CmdDoneMsg     struct{ Err error }
	CmdOutputMsg   struct{ Output string }
	ContainerMsg   struct{ Running bool }
	WorkspacesMsg  struct{ List map[string]string }
	LogsSnapshotMsg struct {
		Service string
		Output  string
		Err     error
	}
	ContainersReadyMsg struct{}
	ShellQuitMsg       struct{}
)

// MenuItem represents a dashboard menu entry.
type MenuItem struct {
	Title       string
	Desc        string
	Screen      Screen
	Icon        string
	NeedsDocker bool
}

// App is the main Bubble Tea model.
type App struct {
	Styles       Styles
	Screen       Screen
	Width        int
	Height       int
	ProjectRoot  string
	Compose      *docker.Compose
	WorkspaceMgr *workspace.Manager
	State        *config.State

	// Menu
	Menu       []MenuItem
	MenuCursor int

	// Forms
	PathInput    textinput.Model
	CommandInput textinput.Model
	AliasInput   textinput.Model

	// Lists
	Workspaces       map[string]string
	WorkspaceList    []string
	DetectedProjects []string // from PROJECT_PATHS env
	WSCursor         int

	// Confirmation
	ConfirmAction func() tea.Cmd
	ConfirmTitle  string
	ConfirmText   string
	ConfirmCursor int

	// Output
	OutputTitle      string
	OutputText       string
	OutputCmd        *exec.Cmd
	LogsActive       bool         // true when output screen is showing logs
	LogsService      string       // docker compose service key being viewed
	PendingAfterStart func() tea.Cmd // action to run after containers are up

	// Spinner
	Spinner     spinner.Model
	Loading     bool
	LoadingText string

	// Status
	StatusText  string
	StatusError error
	LastRefresh time.Time

	// Direct mode (non-TUI)
	DirectMode bool
	DirectArgs []string
}

// NewApp creates a new TUI app.
func NewApp(projectRoot string, directMode bool, directArgs []string) (*App, error) {
	compose, err := docker.NewCompose(projectRoot)
	if err != nil {
		return nil, err
	}

	s := DefaultStyles()

	pathInput := textinput.New()
	pathInput.Placeholder = "Enter path or alias..."
	pathInput.Focus()
	pathInput.Width = 60
	pathInput.PromptStyle = s.Key

	cmdInput := textinput.New()
	cmdInput.Placeholder = "Enter command to execute..."
	cmdInput.Focus()
	cmdInput.Width = 60
	cmdInput.PromptStyle = s.Key

	aliasInput := textinput.New()
	aliasInput.Placeholder = "Enter alias name..."
	aliasInput.Focus()
	aliasInput.Width = 60
	aliasInput.PromptStyle = s.Key

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(Primary)

	menu := []MenuItem{
		{Title: "Run Project", Desc: "Start sandbox with a project mounted", Screen: ScreenRun, Icon: "▶", NeedsDocker: true},
		{Title: "Execute Command", Desc: "Run a command inside the container", Screen: ScreenExec, Icon: "⚡", NeedsDocker: true},
		{Title: "Open Shell", Desc: "Interactive shell in the container", Screen: ScreenOutput, Icon: "🐚", NeedsDocker: true},
		{Title: "Workspaces", Desc: "Manage favorite workspaces", Screen: ScreenWorkspaces, Icon: "📁", NeedsDocker: false},
		{Title: "Status", Desc: "View container and mount status", Screen: ScreenStatus, Icon: "ℹ", NeedsDocker: true},
		{Title: "Logs", Desc: "Tail logs from services", Screen: ScreenLogs, Icon: "📋", NeedsDocker: true},
		{Title: "Run Tests", Desc: "Guardian & security test suites", Screen: ScreenTests, Icon: "🧪", NeedsDocker: true},
		{Title: "Stop", Desc: "Stop all containers", Screen: ScreenConfirm, Icon: "⏹", NeedsDocker: true},
		{Title: "Clean", Desc: "Stop and remove everything", Screen: ScreenConfirm, Icon: "🗑", NeedsDocker: true},
		{Title: "Quit", Desc: "Exit Alcatraz CLI", Screen: ScreenDashboard, Icon: "👋", NeedsDocker: false},
	}

	app := &App{
		Styles:        s,
		Screen:        ScreenSplash,
		ProjectRoot:   projectRoot,
		Compose:       compose,
		WorkspaceMgr:  workspace.NewManager(projectRoot),
		State:         config.NewState(projectRoot),
		Menu:          menu,
		MenuCursor:    0,
		PathInput:     pathInput,
		CommandInput:  cmdInput,
		AliasInput:    aliasInput,
		Spinner:       sp,
		DirectMode:    directMode,
		DirectArgs:    directArgs,
	}

	return app, nil
}

// Init implements tea.Model.
func (a *App) Init() tea.Cmd {
	if a.DirectMode {
		return a.runDirectCommand()
	}
	return tea.Batch(
		spinner.Tick,
		a.splashTick(),
	)
}

// Update implements tea.Model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// ── 1. Update inputs FIRST (so they receive all keystrokes) ──
	if a.Screen == ScreenRun {
		var c tea.Cmd
		a.PathInput, c = a.PathInput.Update(msg)
		cmds = append(cmds, c)
	}
	if a.Screen == ScreenExec {
		var c tea.Cmd
		a.CommandInput, c = a.CommandInput.Update(msg)
		cmds = append(cmds, c)
	}
	if a.Screen == ScreenWorkspaces {
		var c tea.Cmd
		a.AliasInput, c = a.AliasInput.Update(msg)
		cmds = append(cmds, c)
	}
	if a.Loading {
		var c tea.Cmd
		a.Spinner, c = a.Spinner.Update(msg)
		cmds = append(cmds, c)
	}

	// ── 2. Global shortcuts ──
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Global quit
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return a, tea.Quit
		}
		// Global back
		if msg.String() == "esc" {
			if a.Screen != ScreenDashboard && a.Screen != ScreenSplash {
				a.Screen = ScreenDashboard
				a.StatusError = nil
				a.Loading = false
				a.PendingAfterStart = nil
				return a, nil
			}
		}

		// ── 3. Screen-specific key handlers ──
		consumed := false
		var handlerCmd tea.Cmd
		switch a.Screen {
		case ScreenSplash:
			a.Screen = ScreenDashboard
			cmds = append(cmds, a.refreshWorkspaces())
			consumed = true

		case ScreenDashboard:
			consumed, handlerCmd = a.handleDashboardKeys(msg)

		case ScreenRun:
			consumed, handlerCmd = a.handleRunKeys(msg)

		case ScreenExec:
			consumed, handlerCmd = a.handleExecKeys(msg)

		case ScreenWorkspaces:
			consumed, handlerCmd = a.handleWorkspacesKeys(msg)

		case ScreenStatus:
			consumed, handlerCmd = a.handleStatusKeys(msg)

		case ScreenLogs:
			consumed, handlerCmd = a.handleLogsKeys(msg)

		case ScreenTests:
			consumed, handlerCmd = a.handleTestsKeys(msg)

		case ScreenConfirm:
			consumed, handlerCmd = a.handleConfirmKeys(msg)

		case ScreenOutput:
			consumed, handlerCmd = a.handleOutputKeys(msg)
		}

		if consumed {
			if handlerCmd != nil {
				cmds = append(cmds, handlerCmd)
			}
			return a, tea.Batch(cmds...)
		}

	case tea.WindowSizeMsg:
		a.Width = msg.Width
		a.Height = msg.Height
		return a, tea.Batch(cmds...)

	case TickMsg:
		if a.Screen == ScreenSplash {
			a.Screen = ScreenDashboard
			cmds = append(cmds, a.refreshWorkspaces())
		} else {
			cmds = append(cmds, a.splashTick())
		}
		return a, tea.Batch(cmds...)

	case WorkspacesMsg:
		a.Workspaces = msg.List
		a.WorkspaceList = make([]string, 0, len(msg.List))
		for name := range msg.List {
			a.WorkspaceList = append(a.WorkspaceList, name)
		}
		return a, tea.Batch(cmds...)

	case ContainersReadyMsg:
		a.Loading = false
		if a.PendingAfterStart != nil {
			pending := a.PendingAfterStart
			a.PendingAfterStart = nil
			cmds = append(cmds, pending())
		}
		return a, tea.Batch(cmds...)

	case ShellQuitMsg:
		return a, tea.Quit

	case CmdDoneMsg:
		a.Loading = false
		a.PendingAfterStart = nil
		if msg.Err != nil {
			a.OutputText = fmt.Sprintf("Error: %v", msg.Err)
		} else {
			a.Screen = ScreenDashboard
			return a, a.refreshWorkspaces()
		}
		return a, tea.Batch(cmds...)

	case CmdOutputMsg:
		a.OutputText = msg.Output
		a.Loading = false
		return a, tea.Batch(cmds...)

	case LogsSnapshotMsg:
		a.Loading = false
		a.LogsActive = true
		a.LogsService = msg.Service
		if msg.Err != nil {
			a.OutputText = fmt.Sprintf("Error fetching logs: %v", msg.Err)
		} else {
			a.OutputText = msg.Output
		}
		a.Screen = ScreenOutput
		return a, tea.Batch(cmds...)
	}

	return a, tea.Batch(cmds...)
}

// View implements tea.Model.
func (a *App) View() string {
	if a.DirectMode {
		return a.OutputText
	}

	var content string
	switch a.Screen {
	case ScreenSplash:
		content = a.viewSplash()
	case ScreenDashboard:
		content = a.viewDashboard()
	case ScreenRun:
		content = a.viewRun()
	case ScreenExec:
		content = a.viewExec()
	case ScreenWorkspaces:
		content = a.viewWorkspaces()
	case ScreenStatus:
		content = a.viewStatus()
	case ScreenLogs:
		content = a.viewLogs()
	case ScreenTests:
		content = a.viewTests()
	case ScreenConfirm:
		content = a.viewConfirm()
	case ScreenOutput:
		content = a.viewOutput()
	}

	header := a.Styles.Header.Render(LogoSmall())
	footer := a.Styles.Footer.Render("↑/↓ navigate • enter select • esc back • q quit")

	return lipgloss.JoinVertical(lipgloss.Left, header, content, footer)
}

// --- Helpers ---

func (a *App) splashTick() tea.Cmd {
	return tea.Tick(1200*time.Millisecond, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

func (a *App) refreshWorkspaces() tea.Cmd {
	return func() tea.Msg {
		ws, _ := a.WorkspaceMgr.Load()
		a.DetectedProjects = config.LoadProjectPaths(a.ProjectRoot)
		return WorkspacesMsg{List: ws}
	}
}

func (a *App) runCmd(cmd *exec.Cmd, title string) tea.Cmd {
	a.OutputCmd = cmd
	a.OutputTitle = title
	a.OutputText = ""
	a.Screen = ScreenOutput
	a.Loading = true
	a.LoadingText = title

	return func() tea.Msg {
		out, err := cmd.CombinedOutput()
		if err != nil {
			return CmdDoneMsg{Err: fmt.Errorf("%s\n%s", err, strings.TrimSpace(string(out)))}
		}
		return CmdDoneMsg{Err: nil}
	}
}

func (a *App) runDirectCommand() tea.Cmd {
	return nil
}

// ResolveProjectRoot finds the alcatraz project root.
// Priority: ALCATRAZ_ROOT env var (set by the wrapper script) → walk up from cwd.
func ResolveProjectRoot() string {
	if root := os.Getenv("ALCATRAZ_ROOT"); root != "" {
		if _, err := os.Stat(filepath.Join(root, "docker-compose.go.yml")); err == nil {
			return root
		}
	}

	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "docker-compose.go.yml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	dir, _ = os.Getwd()
	return dir
}

// CheckDocker verifies docker is available.
func CheckDocker() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker not found in PATH")
	}
	return nil
}

// ── Command implementations ──

func (a *App) doRun(path string) tea.Cmd {
	if path == "" {
		envPath := config.LoadEnvWorkspace(a.ProjectRoot)
		if envPath != "" {
			path = envPath
		} else {
			path = a.State.GetWorkspace()
		}
	} else {
		if resolved, ok := a.WorkspaceMgr.Resolve(path); ok {
			path = resolved
		}
	}
	if path == "" {
		path = filepath.Join(a.ProjectRoot, "project")
	}
	if _, err := os.Stat(path); err != nil {
		os.MkdirAll(path, 0755)
	}
	absPath, _ := filepath.Abs(path)
	prevWorkspace := a.State.GetWorkspace()
	a.State.SetWorkspace(absPath)
	docker.EnsureContextDir(a.ProjectRoot)

	extraPaths := config.LoadProjectPaths(a.ProjectRoot)
	_ = a.Compose.GenerateOverride(absPath, extraPaths)

	name := filepath.Base(absPath)
	if ws, _ := a.WorkspaceMgr.Load(); ws[name] == "" {
		_ = a.WorkspaceMgr.Save(name, absPath)
	}

	if a.Compose.IsRunning("alcatraz") {
		if prevWorkspace == absPath {
			a.Screen = ScreenDashboard
			return nil
		}
		a.Compose.Down(false).Run()
	}

	imageExists := exec.Command("docker", "image", "inspect", "alcatraz:latest").Run() == nil
	var cmd *exec.Cmd
	if !imageExists {
		cmd = a.Compose.Up(false, true)
	} else {
		cmd = a.Compose.Up(true, false)
	}
	return a.runCmd(cmd, "Starting Alcatraz...")
}

func (a *App) doExec(cmdStr string) tea.Cmd {
	envArgs := config.CollectAPIEnvArgs()
	cmd := a.Compose.Exec("alcatraz", cmdStr, envArgs...)
	return a.runCmd(cmd, fmt.Sprintf("exec: %s", cmdStr))
}

// ensureRunning verifica se os containers estão rodando e sobe se não estiverem,
// executando `then` quando estiverem prontos.
func (a *App) ensureRunning(then func() tea.Cmd) tea.Cmd {
	if a.Compose.IsRunning("alcatraz") {
		return then()
	}

	a.PendingAfterStart = then
	a.OutputTitle = "⚡  Iniciando Alcatraz..."
	a.OutputText = ""
	a.Loading = true
	a.LoadingText = "Subindo containers..."
	a.Screen = ScreenOutput

	ws := a.State.GetWorkspace()
	projectRoot := a.ProjectRoot
	extraPaths := config.LoadProjectPaths(a.ProjectRoot)
	compose := a.Compose

	return func() tea.Msg {
		if ws == "" {
			ws = filepath.Join(projectRoot, "project")
		}
		docker.EnsureContextDir(projectRoot)
		if err := compose.GenerateOverride(ws, extraPaths); err != nil {
			return CmdDoneMsg{Err: fmt.Errorf("falha ao configurar workspace: %v", err)}
		}
		imageExists := exec.Command("docker", "image", "inspect", "alcatraz:latest").Run() == nil
		var cmd *exec.Cmd
		if !imageExists {
			cmd = compose.Up(false, true)
		} else {
			cmd = compose.Up(true, false)
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			return CmdDoneMsg{Err: fmt.Errorf("falha ao subir containers:\n%s", strings.TrimSpace(string(out)))}
		}
		return ContainersReadyMsg{}
	}
}

// doShellQuit grava o next-action para shell e sai do TUI. O wrapper script
// abre o shell interativo depois que o TUI encerra.
func (a *App) doShellQuit(path string) func() tea.Cmd {
	return func() tea.Cmd {
		return func() tea.Msg {
			if path == "" {
				path = filepath.Join(a.ProjectRoot, "project")
			}
			config.WriteNextAction(a.ProjectRoot, "shell", path)
			return ShellQuitMsg{}
		}
	}
}

func (a *App) doTestGuardian() tea.Cmd {
	cmd := exec.Command("bash", filepath.Join(a.ProjectRoot, "test-guardian.sh"))
	cmd.Dir = a.ProjectRoot
	return a.runCmd(cmd, "Running Data Guardian tests...")
}

func (a *App) doTestSecurity() tea.Cmd {
	cmd := exec.Command("bash", filepath.Join(a.ProjectRoot, "test-security.sh"))
	cmd.Dir = a.ProjectRoot
	return a.runCmd(cmd, "Running Security tests...")
}
