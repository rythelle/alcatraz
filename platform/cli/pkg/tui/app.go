package tui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/alcatraz/alcatraz/cli/internal/config"
	"github.com/alcatraz/alcatraz/cli/internal/docker"
	"github.com/alcatraz/alcatraz/cli/internal/modules"
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
	ScreenSpawn
	ScreenWorkspaces
	ScreenStatus
	ScreenLogs
	ScreenTests
	ScreenGuard
	ScreenConfirm
	ScreenOutput
	ScreenSessions
	ScreenCheckpoints
	ScreenModules
)

// Msg types
type (
	TickMsg         time.Time
	CmdDoneMsg      struct{ Err error }
	CmdOutputMsg    struct{ Output string }
	CmdStreamMsg    struct{ Line string }
	CmdFinishedMsg  struct{ Err error }
	ContainerMsg    struct{ Running bool }
	WorkspacesMsg   struct{ List map[string]string }
	LogsSnapshotMsg struct {
		Service string
		Output  string
		Err     error
	}
	ContainersReadyMsg   struct{}
	SessionsLoadedMsg    struct{ Items []SessionItem }
	CheckpointsLoadedMsg struct{ Output string }
	ContainersUpMsg      struct {
		Cmd       *exec.Cmd
		Title     string
		OnSuccess tea.Msg
	}
	ShellQuitMsg struct{}
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

	// Modules
	ModuleState   map[string]bool
	ModulesCursor int
	ModulesNotice string

	// Forms
	PathInput     textinput.Model
	CommandInput  textinput.Model
	AliasInput    textinput.Model
	RollbackInput textinput.Model
	SpawnInput    textinput.Model
	SpawnAgentIdx int

	// Guard
	GuardInput   textinput.Model
	GuardStep    guardStep
	GuardDraft   guardRule
	GuardFile    *guardFile
	GuardLoadErr error
	GuardCursor  int
	GuardNotice  string

	// Lists
	Workspaces       map[string]string
	WorkspaceList    []string
	DetectedProjects []string // from PROJECT_PATHS env
	WSCursor         int
	SessionItems     []SessionItem
	SessionCursor    int
	SessionScroll    int // index of the first visible row

	// Confirmation
	ConfirmAction func() tea.Cmd
	ConfirmTitle  string
	ConfirmText   string
	ConfirmCursor int

	// Output
	OutputTitle       string
	OutputText        string
	OutputCmd         *exec.Cmd
	StreamCh          chan tea.Msg   // live output of a streaming command
	LogsActive        bool           // true when output screen is showing logs
	LogsService       string         // docker compose service key being viewed
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

	rollbackInput := textinput.New()
	rollbackInput.Placeholder = "checkpoint # or hash (empty = latest)"
	rollbackInput.Width = 40
	rollbackInput.PromptStyle = s.Key

	guardInput := textinput.New()
	guardInput.Width = 60
	guardInput.PromptStyle = s.Key

	spawnInput := textinput.New()
	spawnInput.Placeholder = "Describe the exploration task..."
	spawnInput.Width = 60
	spawnInput.PromptStyle = s.Key

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(Primary)

	modState := modules.Resolve(projectRoot)

	app := &App{
		Styles:        s,
		Screen:        ScreenSplash,
		ProjectRoot:   projectRoot,
		Compose:       compose,
		WorkspaceMgr:  workspace.NewManager(projectRoot),
		State:         config.NewState(projectRoot),
		ModuleState:   modState,
		MenuCursor:    0,
		PathInput:     pathInput,
		CommandInput:  cmdInput,
		AliasInput:    aliasInput,
		RollbackInput: rollbackInput,
		GuardInput:    guardInput,
		SpawnInput:    spawnInput,
		Spinner:       sp,
		DirectMode:    directMode,
		DirectArgs:    directArgs,
	}
	app.rebuildMenu()

	return app, nil
}

// rebuildMenu constructs the dashboard menu from the current module state, so
// OFF modules' entries don't appear (the TUI equivalent of hiding CLI
// commands). Called at startup and after a toggle on the Modules screen.
func (a *App) rebuildMenu() {
	on := func(key string) bool { return a.ModuleState[key] }
	menu := []MenuItem{
		{Title: "Run Project", Desc: "Mount a project and start containers (restarts if project changes)", Screen: ScreenRun, Icon: "▶", NeedsDocker: true},
		{Title: "Execute Command", Desc: "Run a one-off command inside the running container", Screen: ScreenExec, Icon: "⚡", NeedsDocker: true},
	}
	if on("spawn") {
		menu = append(menu, MenuItem{Title: "Spawn", Desc: "Run a task in a disposable sibling sandbox (read-only project); saves the result", Screen: ScreenSpawn, Icon: "🧬", NeedsDocker: true})
	}
	menu = append(menu,
		MenuItem{Title: "Open Shell", Desc: "Interactive shell — starts containers automatically if needed", Screen: ScreenOutput, Icon: "🐚", NeedsDocker: true},
		MenuItem{Title: "Workspaces", Desc: "Switch the active project  (s = open shell; restarts only if the project isn't mounted yet)", Screen: ScreenWorkspaces, Icon: "📁", NeedsDocker: false},
		MenuItem{Title: "Status", Desc: "Container status, active workspace and mounted volumes", Screen: ScreenStatus, Icon: "ℹ", NeedsDocker: false},
	)
	if on("stats") {
		menu = append(menu, MenuItem{Title: "Stats", Desc: "Token usage/cost per day and model, metered by the Guard", Screen: ScreenOutput, Icon: "📊", NeedsDocker: true})
	}
	if on("sessions") {
		menu = append(menu, MenuItem{Title: "Sessions", Desc: "Resumable AI conversations — navigate the list and reopen one in a shell", Screen: ScreenSessions, Icon: "💬", NeedsDocker: true})
	}
	if on("checkpoints") {
		menu = append(menu, MenuItem{Title: "Checkpoints", Desc: "Workspace snapshots — browse and roll back right here", Screen: ScreenCheckpoints, Icon: "📸", NeedsDocker: false})
	}
	menu = append(menu,
		MenuItem{Title: "Logs", Desc: "View last 200 lines of logs per service", Screen: ScreenLogs, Icon: "📋", NeedsDocker: true},
		MenuItem{Title: "Guard", Desc: "Guard rules: custom redactions, allowlist, markers, test & audit", Screen: ScreenGuard, Icon: "🛡", NeedsDocker: false},
		MenuItem{Title: "Run Tests", Desc: "Guard & security test suites", Screen: ScreenTests, Icon: "🧪", NeedsDocker: true},
		MenuItem{Title: "Modules", Desc: "Turn optional features on/off — writes the .env module block", Screen: ScreenModules, Icon: "🧩", NeedsDocker: false},
		MenuItem{Title: "Rebuild & Run", Desc: "Force an image rebuild and restart — no data is lost (volumes persist)", Screen: ScreenConfirm, Icon: "🔄", NeedsDocker: true},
		MenuItem{Title: "Stop", Desc: "Stop all containers (data is preserved)", Screen: ScreenConfirm, Icon: "⏹", NeedsDocker: true},
		MenuItem{Title: "Clean", Desc: "Stop containers and remove all volumes (destructive)", Screen: ScreenConfirm, Icon: "🗑", NeedsDocker: true},
		MenuItem{Title: "Quit", Desc: "Exit Alcatraz CLI", Screen: ScreenDashboard, Icon: "👋", NeedsDocker: false},
	)
	a.Menu = menu
	if a.MenuCursor >= len(menu) {
		a.MenuCursor = len(menu) - 1
	}
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
	if a.Screen == ScreenSpawn {
		var c tea.Cmd
		a.SpawnInput, c = a.SpawnInput.Update(msg)
		cmds = append(cmds, c)
	}
	if a.Screen == ScreenWorkspaces {
		var c tea.Cmd
		a.AliasInput, c = a.AliasInput.Update(msg)
		cmds = append(cmds, c)
	}
	if a.Screen == ScreenCheckpoints {
		var c tea.Cmd
		a.RollbackInput, c = a.RollbackInput.Update(msg)
		cmds = append(cmds, c)
	}
	if a.Screen == ScreenGuard && a.guardInputActive() {
		var c tea.Cmd
		a.GuardInput, c = a.GuardInput.Update(msg)
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

		case ScreenSpawn:
			consumed, handlerCmd = a.handleSpawnKeys(msg)

		case ScreenWorkspaces:
			consumed, handlerCmd = a.handleWorkspacesKeys(msg)

		case ScreenStatus:
			consumed, handlerCmd = a.handleStatusKeys(msg)

		case ScreenLogs:
			consumed, handlerCmd = a.handleLogsKeys(msg)

		case ScreenTests:
			consumed, handlerCmd = a.handleTestsKeys(msg)

		case ScreenGuard:
			consumed, handlerCmd = a.handleGuardKeys(msg)

		case ScreenConfirm:
			consumed, handlerCmd = a.handleConfirmKeys(msg)

		case ScreenOutput:
			consumed, handlerCmd = a.handleOutputKeys(msg)

		case ScreenSessions:
			consumed, handlerCmd = a.handleSessionsKeys(msg)

		case ScreenCheckpoints:
			consumed, handlerCmd = a.handleCheckpointsKeys(msg)

		case ScreenModules:
			consumed, handlerCmd = a.handleModulesKeys(msg)
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
		a.StreamCh = nil
		if a.PendingAfterStart != nil {
			pending := a.PendingAfterStart
			a.PendingAfterStart = nil
			cmds = append(cmds, pending())
		}
		return a, tea.Batch(cmds...)

	case ShellQuitMsg:
		return a, tea.Quit

	case SessionsLoadedMsg:
		a.Loading = false
		a.SessionItems = msg.Items
		a.SessionCursor = 0
		a.SessionScroll = 0
		return a, tea.Batch(cmds...)

	case CheckpointsLoadedMsg:
		a.Loading = false
		a.OutputText = msg.Output
		return a, tea.Batch(cmds...)

	case CmdDoneMsg:
		a.Loading = false
		a.PendingAfterStart = nil
		a.StreamCh = nil
		if msg.Err != nil {
			if a.OutputText == "" {
				a.OutputText = fmt.Sprintf("Error: %v", msg.Err)
			} else {
				a.OutputText += fmt.Sprintf("\n✗ FAILED: %v\n", msg.Err)
			}
		} else {
			a.Screen = ScreenDashboard
			return a, a.refreshWorkspaces()
		}
		return a, tea.Batch(cmds...)

	case ContainersUpMsg:
		a.Loading = true
		return a, a.runStreaming(msg.Cmd, msg.Title, func(err error) tea.Msg {
			if err != nil {
				return CmdDoneMsg{Err: fmt.Errorf("failed to start containers: %v", err)}
			}
			_ = a.Compose.CodexSkillsInit().Run()
			return msg.OnSuccess
		})

	case CmdOutputMsg:
		a.OutputText = msg.Output
		a.Loading = false
		return a, tea.Batch(cmds...)

	case CmdStreamMsg:
		a.OutputText += msg.Line + "\n"
		// keep memory bounded on long builds; the view shows the tail anyway
		if len(a.OutputText) > 64*1024 {
			a.OutputText = a.OutputText[len(a.OutputText)-48*1024:]
		}
		cmds = append(cmds, a.nextStreamMsg())
		return a, tea.Batch(cmds...)

	case CmdFinishedMsg:
		a.Loading = false
		a.StreamCh = nil
		if msg.Err != nil {
			a.OutputText += "\n✗ FAILED: " + msg.Err.Error() + "\n"
		} else {
			a.OutputText += "\n✓ Done — everything finished successfully. Press ESC to go back.\n"
		}
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
	case ScreenSpawn:
		content = a.viewSpawn()
	case ScreenWorkspaces:
		content = a.viewWorkspaces()
	case ScreenStatus:
		content = a.viewStatus()
	case ScreenLogs:
		content = a.viewLogs()
	case ScreenTests:
		content = a.viewTests()
	case ScreenGuard:
		content = a.viewGuard()
	case ScreenConfirm:
		content = a.viewConfirm()
	case ScreenOutput:
		content = a.viewOutput()
	case ScreenSessions:
		content = a.viewSessions()
	case ScreenCheckpoints:
		content = a.viewCheckpoints()
	case ScreenModules:
		content = a.viewModules()
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
	return a.runStreaming(cmd, title, func(err error) tea.Msg {
		return CmdDoneMsg{Err: err}
	})
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

// resolveRunPath turns the Run screen's input (empty, a saved alias, or a
// path) into a concrete workspace path, matching the precedence the CLI uses.
func (a *App) resolveRunPath(path string) string {
	if path == "" {
		if envPath := config.LoadEnvWorkspace(a.ProjectRoot); envPath != "" {
			return envPath
		}
		if ws := a.State.GetWorkspace(); ws != "" {
			return ws
		}
		return filepath.Join(a.ProjectRoot, "project")
	}
	if resolved, ok := a.WorkspaceMgr.Resolve(path); ok {
		return resolved
	}
	return path
}

// doRun resolves the target path and starts the stack. A running container
// can't gain a new bind mount, so mounting a project it doesn't already have
// requires a full restart (which closes open shells) — in that case we warn
// and confirm first. If the project is already mounted (active workspace or a
// PROJECT_PATHS entry), we skip the restart entirely.
func (a *App) doRun(path string) tea.Cmd {
	path = a.resolveRunPath(path)
	if _, err := os.Stat(path); err != nil {
		os.MkdirAll(path, 0755)
	}
	absPath, _ := filepath.Abs(path)
	name := filepath.Base(absPath)
	workdir := "/workspace/projects/" + name

	needsRestart := a.Compose.IsRunning("alcatraz") && !a.Compose.PathMounted(workdir)
	if needsRestart {
		a.ConfirmTitle = "Restart containers to mount this project?"
		a.ConfirmText = fmt.Sprintf(
			"“%s” isn't mounted in the running container. Docker can't add a mount\n"+
				"to a live container, so mounting it needs a full restart — which CLOSES\n"+
				"every open shell session.\n\n"+
				"Before restarting, Mega Brain flushes each project's recorded context\n"+
				"to disk and marks the open task PAUSED (mega-brain pause-all), so you\n"+
				"can pick each one back up with 'mega-brain resume'. Unsaved in-flight\n"+
				"chat that no hook has captured yet is not recovered.\n\n"+
				"Tip: add this path to PROJECT_PATHS in .env to mount it at startup and\n"+
				"switch to it later without any restart.\n\nContinue?", name)
		a.ConfirmCursor = 1
		a.ConfirmAction = func() tea.Cmd { return a.doRunStart(absPath, true) }
		a.Screen = ScreenConfirm
		return nil
	}
	return a.doRunStart(absPath, false)
}

// doRunStart configures the workspace and brings the stack up. When restart is
// true it snapshots and tears the running container down first; otherwise, if
// the container is already up (project already mounted) it is a no-op.
func (a *App) doRunStart(absPath string, restart bool) tea.Cmd {
	a.OutputTitle = "▶  Starting Alcatraz..."
	a.OutputText = ""
	a.Loading = true
	a.LoadingText = "Preparing workspace..."
	a.Screen = ScreenOutput

	return func() tea.Msg {
		a.State.SetWorkspace(absPath)
		docker.EnsureContextDir(a.ProjectRoot)

		extraPaths := config.LoadProjectPaths(a.ProjectRoot)
		if err := a.Compose.GenerateOverride(absPath, extraPaths); err != nil {
			return CmdDoneMsg{Err: fmt.Errorf("failed to configure workspace: %v", err)}
		}

		name := filepath.Base(absPath)
		if ws, _ := a.WorkspaceMgr.Load(); ws[name] == "" {
			_ = a.WorkspaceMgr.Save(name, absPath)
		}

		running := a.Compose.IsRunning("alcatraz")
		if running && !restart {
			// Already up and this project is mounted — nothing to restart, but
			// still re-link: this is the path a user hits after adding a skill
			// to the host.
			_ = a.Compose.CodexSkillsInit().Run()
			return CmdDoneMsg{Err: nil}
		}
		if restart {
			// Snapshot every project before the restart so open shells' work
			// survives (resume with `mega-brain resume`).
			_ = a.Compose.PauseAll().Run()
			if out, err := a.Compose.Down(false).CombinedOutput(); err != nil {
				return CmdDoneMsg{Err: fmt.Errorf("failed to stop containers:\n%s", strings.TrimSpace(string(out)))}
			}
		}

		imageExists := exec.Command("docker", "image", "inspect", "alcatraz:latest").Run() == nil
		var cmd *exec.Cmd
		if !imageExists {
			cmd = a.Compose.Up(false, true)
		} else {
			cmd = a.Compose.Up(true, false)
		}
		return ContainersUpMsg{Cmd: cmd, Title: "▶  Starting Alcatraz...", OnSuccess: CmdDoneMsg{Err: nil}}
	}
}

func (a *App) doExec(cmdStr string) tea.Cmd {
	envArgs := config.CollectAPIEnvArgs()
	cmd := a.Compose.Exec("alcatraz", cmdStr, envArgs...)
	return a.runCmd(cmd, fmt.Sprintf("exec: %s", cmdStr))
}

// spawnAgentChoices is the agent selector shown on the Spawn screen; it mirrors
// the allowlist enforced by the `spawn` command.
var spawnAgentChoices = []string{"claude", "codex", "gemini", "opencode"}

// runSpawn shells out to this same binary's `spawn` subcommand and streams its
// output live. Reusing the subcommand keeps the TUI on the exact hardened path
// the CLI uses — no duplicated docker logic in the TUI package.
func (a *App) runSpawn(agent, task string) tea.Cmd {
	self, err := os.Executable()
	if err != nil || self == "" {
		self = os.Args[0]
	}
	cmd := exec.Command(self, "spawn", "-a", agent, task)
	cmd.Env = os.Environ()
	if a.ProjectRoot != "" {
		cmd.Env = append(cmd.Env, "ALCATRAZ_ROOT="+a.ProjectRoot)
	}
	return a.runCmdStreaming(cmd, fmt.Sprintf("🧬  Spawn (%s): %s", agent, task))
}

// ensureRunning checks whether the containers are running and starts them if
// not, executing `then` once they are ready.
func (a *App) ensureRunning(then func() tea.Cmd) tea.Cmd {
	return a.ensureRunningImpl(then, false)
}

// ensureRunningImpl is like ensureRunning but can force a container restart when
// the active workspace has changed and its path isn't mounted yet. When the
// container is already running and a restart is needed, it asks for confirmation
// first — a restart closes every open shell session — and snapshots all projects
// before tearing the container down.
func (a *App) ensureRunningImpl(then func() tea.Cmd, forceRestart bool) tea.Cmd {
	running := a.Compose.IsRunning("alcatraz")
	if running && !forceRestart {
		return then()
	}

	// Restarting a live container is destructive to open shells: confirm first.
	if running {
		a.ConfirmTitle = "Restart containers?"
		a.ConfirmText = "This project isn't mounted in the running container, so mounting\n" +
			"it needs a full restart — which CLOSES every open shell session.\n\n" +
			"Before restarting, Mega Brain flushes each project's recorded context\n" +
			"to disk and marks the open task PAUSED (mega-brain pause-all) — resume\n" +
			"with 'mega-brain resume'. Unsaved in-flight chat is not recovered.\n\nContinue?"
		a.ConfirmCursor = 1
		then := then
		a.ConfirmAction = func() tea.Cmd { return a.startContainers(then, true) }
		a.Screen = ScreenConfirm
		return nil
	}

	// Container is down: nothing to lose, just start it.
	return a.startContainers(then, false)
}

// startContainers regenerates the compose override and brings the stack up,
// running `then` once containers are ready. When restart is true the container
// is already running: it snapshots every project (mega-brain pause-all) and
// takes the stack down first. This is the single place that performs down/up.
func (a *App) startContainers(then func() tea.Cmd, restart bool) tea.Cmd {
	a.PendingAfterStart = then
	if restart {
		a.OutputTitle = "⚡  Restarting Alcatraz..."
		a.LoadingText = "Saving context (mega-brain pause-all) and restarting..."
	} else {
		a.OutputTitle = "⚡  Starting Alcatraz..."
		a.LoadingText = "Starting containers..."
	}
	a.OutputText = ""
	a.Loading = true
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
			return CmdDoneMsg{Err: fmt.Errorf("failed to configure workspace: %v", err)}
		}
		if restart {
			// Best-effort: snapshot every project before the container dies.
			_ = compose.PauseAll().Run()
			if out, err := compose.Down(false).CombinedOutput(); err != nil {
				return CmdDoneMsg{Err: fmt.Errorf("failed to stop containers:\n%s", strings.TrimSpace(string(out)))}
			}
		}
		imageExists := exec.Command("docker", "image", "inspect", "alcatraz:latest").Run() == nil
		var cmd *exec.Cmd
		if !imageExists {
			cmd = compose.Up(false, true)
		} else {
			cmd = compose.Up(true, false)
		}
		return ContainersUpMsg{Cmd: cmd, Title: a.OutputTitle, OnSuccess: ContainersReadyMsg{}}
	}
}

// doShellQuit records the shell next-action and exits the TUI. The wrapper
// script opens the interactive shell after the TUI terminates.
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

// runStreaming runs a command showing its output LIVE on the output screen.
// onFinish is called when the command exits and its return value is dispatched
// back into the Bubble Tea update loop. Long operations (docker up, builds,
// exec) use this so the user can see what is happening instead of staring at
// a blind spinner.
func (a *App) runStreaming(cmd *exec.Cmd, title string, onFinish func(error) tea.Msg) tea.Cmd {
	a.OutputCmd = cmd
	a.OutputTitle = title
	a.OutputText = ""
	a.LogsActive = false
	a.Screen = ScreenOutput
	a.Loading = true
	a.LoadingText = title

	ch := make(chan tea.Msg, 64)
	a.StreamCh = ch

	go func() {
		pr, pw := io.Pipe()
		cmd.Stdout = pw
		cmd.Stderr = pw
		if err := cmd.Start(); err != nil {
			ch <- onFinish(err)
			close(ch)
			return
		}
		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
			pw.Close()
		}()
		scanner := bufio.NewScanner(pr)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			ch <- CmdStreamMsg{Line: scanner.Text()}
		}
		ch <- onFinish(<-done)
		close(ch)
	}()

	return a.nextStreamMsg()
}

// runCmdStreaming runs a command with live output and reports success/failure
// on the output screen, leaving the full log visible (used by Spawn).
func (a *App) runCmdStreaming(cmd *exec.Cmd, title string) tea.Cmd {
	return a.runStreaming(cmd, title, func(err error) tea.Msg {
		return CmdFinishedMsg{Err: err}
	})
}

// nextStreamMsg re-subscribes to the stream channel; each received message
// schedules the next one, so lines flow into the view as they arrive.
func (a *App) nextStreamMsg() tea.Cmd {
	ch := a.StreamCh
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

// doSnapshot runs a command to completion and shows its full output on the
// output screen (unlike runCmd, which returns to the dashboard on success).
func (a *App) doSnapshot(title string, cmd *exec.Cmd) tea.Cmd {
	a.OutputTitle = title
	a.OutputText = ""
	a.LogsActive = false
	a.Screen = ScreenOutput
	a.Loading = true
	a.LoadingText = title

	return func() tea.Msg {
		out, err := cmd.CombinedOutput()
		text := strings.TrimSpace(string(out))
		if err != nil && text == "" {
			text = fmt.Sprintf("Error: %v", err)
		}
		return CmdOutputMsg{Output: text}
	}
}

// The stats/sessions/checkpoints logic lives in alcatraz.sh; the TUI shows
// its output the same way the cobra commands delegate to it.
func (a *App) doShellSnapshot(title, action string) tea.Cmd {
	cmd := exec.Command("bash", filepath.Join(a.ProjectRoot, "alcatraz.sh"), action)
	cmd.Dir = a.ProjectRoot
	return a.doSnapshot(title, cmd)
}

func (a *App) doStats() tea.Cmd {
	return a.doShellSnapshot("📊  Token usage (Guard)", "stats")
}

func (a *App) doRebuild() tea.Cmd {
	ws := a.State.GetWorkspace()
	if ws == "" {
		ws = filepath.Join(a.ProjectRoot, "project")
	}
	docker.EnsureContextDir(a.ProjectRoot)
	_ = a.Compose.GenerateOverride(ws, config.LoadProjectPaths(a.ProjectRoot))
	// Not runCmdStreaming: the host's Codex skills are re-linked once the stack
	// is up, because `up` leaves an unchanged container running and the
	// container-side init only runs at boot.
	return a.runStreaming(a.Compose.Up(false, true), "🔄  Rebuilding image and restarting...", func(err error) tea.Msg {
		if err == nil {
			_ = a.Compose.CodexSkillsInit().Run()
		}
		return CmdFinishedMsg{Err: err}
	})
}

func (a *App) doTestGuard() tea.Cmd {
	cmd := exec.Command("bash", filepath.Join(a.ProjectRoot, "test-guard.sh"))
	cmd.Dir = a.ProjectRoot
	return a.runCmd(cmd, "Running Guard tests...")
}

func (a *App) doTestSecurity() tea.Cmd {
	cmd := exec.Command("bash", filepath.Join(a.ProjectRoot, "test-security.sh"))
	cmd.Dir = a.ProjectRoot
	return a.runCmd(cmd, "Running Security tests...")
}
