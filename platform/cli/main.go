package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/alcatraz/alcatraz/cli/internal/config"
	"github.com/alcatraz/alcatraz/cli/internal/docker"
	"github.com/alcatraz/alcatraz/cli/internal/guard"
	"github.com/alcatraz/alcatraz/cli/internal/modules"
	"github.com/alcatraz/alcatraz/cli/internal/workspace"
	"github.com/alcatraz/alcatraz/cli/pkg/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var (
	projectRoot string
	compose     *docker.Compose
	wsMgr       *workspace.Manager
	state       *config.State
)

func init() {
	projectRoot = tui.ResolveProjectRoot()
	var err error
	compose, err = docker.NewCompose(projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	wsMgr = workspace.NewManager(projectRoot)
	state = config.NewState(projectRoot)
	// Create ~/.alcatraz (owned by the user) so the backend's read-only
	// bind-mount source exists before `docker compose up`, avoiding a
	// root-owned directory the CLI later can't write guard rules into.
	_ = guard.EnsureDir()

	// Migration: installs predating modules get the default block injected
	// once, with a notice so nobody loses a feature silently.
	if injected, _ := modules.EnsureBlock(projectRoot); injected {
		fmt.Fprintln(os.Stderr, "ℹ  Module toggles added to .env. Mega Brain, shakedown and spawn are now")
		fmt.Fprintln(os.Stderr, "   opt-in — re-enable with ALCATRAZ_MOD_<NAME>=on (or the TUI Modules screen).")
	}
	// Single resolution point: compute module state once and push it into the
	// process environment so any `docker compose up`/`exec` child (and thus the
	// container) sees the exact values the CLI resolved.
	modules.Export(projectRoot)
}

// gateModule enforces module state at the CLI level: an OFF module's command is
// hidden from --help and, if invoked directly, prints the friendly off-notice
// instead of running. Core/always-on features never pass through here.
func gateModule(cmd *cobra.Command, key string) *cobra.Command {
	if modules.Enabled(projectRoot, key) {
		return cmd
	}
	cmd.Hidden = true
	cmd.RunE = func(*cobra.Command, []string) error {
		fmt.Println(modules.OffMessage(key))
		return nil
	}
	return cmd
}

// gateModuleAny is gateModule for a command shared by several modules — the
// bridge watcher serves both spawn and websearch, so it stays available while
// either one is on, and names the ones that are off when invoked with none.
func gateModuleAny(cmd *cobra.Command, keys ...string) *cobra.Command {
	for _, key := range keys {
		if modules.Enabled(projectRoot, key) {
			return cmd
		}
	}
	cmd.Hidden = true
	cmd.RunE = func(*cobra.Command, []string) error {
		fmt.Printf("all of %s are off — enable one to use this (e.g. alcatraz modules %s on)\n",
			strings.Join(keys, ", "), keys[0])
		return nil
	}
	return cmd
}

func main() {
	var rootCmd = &cobra.Command{
		Use:   "alcatraz-cli",
		Short: "Alcatraz - Isolated Sandbox for AI Tools",
		Long: `Alcatraz CLI - Interactive TUI and command-line interface
for managing the Alcatraz isolated sandbox for AI tools.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// No args → launch TUI
			app, err := tui.NewApp(projectRoot, false, nil)
			if err != nil {
				return err
			}
			p := tea.NewProgram(app, tea.WithAltScreen())
			_, err = p.Run()
			return err
		},
	}

	rootCmd.AddCommand(
		buildCmd(),
		runCmd(),
		saveCmd(),
		listCmd(),
		removeCmd(),
		execCmd(),
		shellCmd(),
		shellRunCmd(),
		gateModule(spawnCmd(), "spawn"),
		gateModuleAny(spawnWatchCmd(), "spawn", "websearch"),
		stopCmd(),
		cleanCmd(),
		statusCmd(),
		gateModule(statsCmd(), "stats"),
		gateModule(sessionsCmd(), "sessions"),
		gateModule(checkpointCmd(), "checkpoints"),
		gateModule(checkpointsCmd(), "checkpoints"),
		gateModule(rollbackCmd(), "checkpoints"),
		resourcesCmd(),
		logsCmd(),
		guardCmd(),
		testGuardCmd(),
		testSecurityCmd(),
		modulesCmd(),
		tuiCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// modulesCmd lists module state and toggles it from the command line. The TUI
// Modules screen is the graphical equivalent; both edit the same .env block.
func modulesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "modules [NAME on|off]",
		Short: "List optional modules and turn them on/off (edits .env)",
		Long: `Show every optional module and whether it is on.

The core — sandbox, Lighthouse and Guard — is always on and is not listed here.
Safety-net modules (checkpoints, sessions, stats) default on; the rest are
opt-in. Toggle one with:  alcatraz modules spawn on

Changes are written to the .env module block. Runtime modules (megabrain,
shakedown, spawn) take effect on the next 'alcatraz run'.`,
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				key := args[0]
				if _, ok := modules.Get(key); !ok {
					return fmt.Errorf("unknown module %q", key)
				}
				on, valid := map[string]bool{"on": true, "off": false}[strings.ToLower(args[1])]
				if !valid {
					return fmt.Errorf("state must be 'on' or 'off', got %q", args[1])
				}
				if err := modules.SetInEnv(projectRoot, key, on); err != nil {
					return err
				}
				fmt.Printf("✓ %s is now %s (applies on next 'alcatraz run')\n", key, args[1])
				return nil
			}
			state := modules.Resolve(projectRoot)
			fmt.Println("Optional modules (core is always on):")
			fmt.Println()
			lastLayer := modules.Layer("")
			for _, m := range modules.All {
				if m.Layer != lastLayer {
					if m.Layer == modules.LayerSafety {
						fmt.Println("  Safety net (on by default):")
					} else {
						fmt.Println("\n  Opt-in (off by default):")
					}
					lastLayer = m.Layer
				}
				mark := "○ off"
				if state[m.Key] {
					mark = "● on "
				}
				fmt.Printf("    %s  %-12s %s\n", mark, m.Key, m.Desc)
			}
			fmt.Println("\n  Toggle:  alcatraz modules <name> on|off")
			return nil
		},
	}
	return cmd
}

func tuiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Launch the interactive TUI",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := tui.NewApp(projectRoot, false, nil)
			if err != nil {
				return err
			}
			p := tea.NewProgram(app, tea.WithAltScreen())
			_, err = p.Run()
			return err
		},
	}
}

func buildCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "build",
		Short: "Build the Docker image",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := compose.Build()
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	}
}

func runCmd() *cobra.Command {
	var rebuild bool
	cmd := &cobra.Command{
		Use:   "run [PATH|ALIAS]",
		Short: "Start the sandbox with a project mounted",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := ""
			if len(args) > 0 {
				path = workspace.NormalizePath(args[0])
			}

			// Resolve
			if path == "" {
				envPath := config.LoadEnvWorkspace(projectRoot)
				if envPath != "" {
					path = envPath
				} else {
					path = state.GetWorkspace()
				}
			} else {
				if resolved, ok := wsMgr.Resolve(path); ok {
					path = resolved
				}
			}

			if path == "" {
				path = filepath.Join(projectRoot, "project")
			}

			if _, err := os.Stat(path); err != nil {
				os.MkdirAll(path, 0755)
			}

			absPath, _ := filepath.Abs(path)
			prevWorkspace := state.GetWorkspace()
			state.SetWorkspace(absPath)
			docker.EnsureContextDir(projectRoot)

			extraPaths := config.LoadProjectPaths(projectRoot)
			if err := compose.GenerateOverride(absPath, extraPaths); err != nil {
				return err
			}

			if len(args) > 0 {
				name := filepath.Base(absPath)
				if ws, _ := wsMgr.Load(); ws[name] == "" {
					_ = wsMgr.Save(name, absPath)
				}
			}

			if compose.IsRunning("alcatraz") && !rebuild {
				if prevWorkspace == absPath {
					fmt.Println("✓ Alcatraz is already running with this project")
					fmt.Printf("  Project: %s -> /workspace\n", absPath)
					return nil
				}
				fmt.Println("Stopping current container to remount...")
				compose.Down(false).Run()
			}

			imageExists := exec.Command("docker", "image", "inspect", "alcatraz:latest").Run() == nil

			var dcCmd *exec.Cmd
			if rebuild || !imageExists {
				dcCmd = compose.Up(false, true)
			} else {
				dcCmd = compose.Up(true, false)
			}
			dcCmd.Stdout = os.Stdout
			dcCmd.Stderr = os.Stderr
			if err := dcCmd.Run(); err != nil {
				return err
			}

			fmt.Println("✓ Alcatraz is running")
			fmt.Printf("  Project: %s -> /workspace\n", absPath)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&rebuild, "rebuild", "b", false, "Force image rebuild")
	return cmd
}

func saveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "save <name> [path]",
		Short: "Save a favorite workspace",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			path := ""
			if len(args) > 1 {
				path = workspace.NormalizePath(args[1])
			} else {
				path = state.GetWorkspace()
			}

			absPath, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			if _, err := os.Stat(absPath); err != nil {
				return fmt.Errorf("directory does not exist: %s", absPath)
			}

			if err := wsMgr.Save(name, absPath); err != nil {
				return err
			}
			fmt.Printf("✓ Workspace '%s' saved -> %s\n", name, absPath)
			return nil
		},
	}
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all workspaces (favorites + PROJECT_PATHS)",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaces, err := wsMgr.Load()
			if err != nil {
				return err
			}
			detected := config.LoadProjectPaths(projectRoot)

			if len(workspaces) == 0 && len(detected) == 0 {
				fmt.Println("No workspaces found.")
				fmt.Println("  Save a favorite:  alcatraz save <name> [path]")
				fmt.Println("  Or set PROJECT_PATHS in .env")
				return nil
			}

			if len(workspaces) > 0 {
				fmt.Println("⭐ Favorite workspaces:")
				fmt.Println("")
				for name, path := range workspaces {
					icon := "✓"
					if _, err := os.Stat(path); err != nil {
						icon = "⚠"
					}
					fmt.Printf("  %s %-18s %s\n", icon, name, path)
				}
			}

			if len(detected) > 0 {
				if len(workspaces) > 0 {
					fmt.Println("")
				}
				fmt.Println("🔍 Detected from PROJECT_PATHS:")
				fmt.Println("")
				for _, path := range detected {
					name := filepath.Base(path)
					icon := "✓"
					if _, err := os.Stat(path); err != nil {
						icon = "⚠"
					}
					fmt.Printf("  %s %-18s %s  [auto]\n", icon, name, path)
				}
			}
			return nil
		},
	}
}

func removeCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "remove <name>",
		Aliases: []string{"rm"},
		Short:   "Remove a favorite workspace",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := wsMgr.Remove(args[0]); err != nil {
				return err
			}
			fmt.Printf("✓ Workspace '%s' removed.\n", args[0])
			return nil
		},
	}
}

func execCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "exec 'command'",
		Short: "Run a command inside the container",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			command := strings.Join(args, " ")
			envArgs := config.CollectAPIEnvArgs()
			c := compose.Exec("alcatraz", command, envArgs...)
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	}
}

// workspaceMounted reports whether workdir exists inside the running alcatraz
// container. Used to detect a state/container mismatch: the saved workspace
// says one project, but the running container was started without it mounted.
func workspaceMounted(workdir string) bool {
	if workdir == "" || workdir == "/workspace" {
		return true
	}
	return compose.ExecRaw("alcatraz", "test", "-d", workdir).Run() == nil
}

// startWithWorkspace regenerates the compose override so absPath is mounted at
// /workspace/projects/<name> and (re)starts the containers. A running container
// cannot gain a new bind mount, so mounting a project always requires a restart.
func startWithWorkspace(absPath string) error {
	docker.EnsureContextDir(projectRoot)
	extraPaths := config.LoadProjectPaths(projectRoot)
	if err := compose.GenerateOverride(absPath, extraPaths); err != nil {
		return err
	}
	if compose.IsRunning("alcatraz") {
		// Snapshot every project before the container dies so in-progress work
		// in any open shell survives (resume with `mega-brain resume`).
		compose.PauseAll().Run()
		compose.Down(false).Run()
	}
	imageExists := exec.Command("docker", "image", "inspect", "alcatraz:latest").Run() == nil
	var dcCmd *exec.Cmd
	if !imageExists {
		dcCmd = compose.Up(false, true)
	} else {
		dcCmd = compose.Up(true, false)
	}
	return dcCmd.Run()
}

func shellCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "shell [PATH|ALIAS]",
		Short: "Open an interactive shell (starts container if needed)",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := ""
			if len(args) > 0 {
				path = workspace.NormalizePath(args[0])
				if resolved, ok := wsMgr.Resolve(path); ok {
					path = resolved
				}
			}

			// Resolve the target workspace: an explicit arg, else the saved one.
			absPath := ""
			if path != "" {
				if _, err := os.Stat(path); err != nil {
					return fmt.Errorf("directory does not exist: %s", path)
				}
				absPath, _ = filepath.Abs(path)
				state.SetWorkspace(absPath)
			} else {
				absPath = state.GetWorkspace()
			}

			workdir := "/workspace"
			if absPath != "" {
				workdir = "/workspace/projects/" + filepath.Base(absPath)
			}

			// (Re)start only when necessary: the container is down, or the target
			// project isn't mounted in the running one (e.g. the workspace was
			// switched via the TUI without a restart). Otherwise reuse the running
			// container so other active shell sessions survive.
			if running := compose.IsRunning("alcatraz"); !running || !workspaceMounted(workdir) {
				if running {
					fmt.Printf("⚠  Restarting to mount %s — other active shell sessions will close.\n", filepath.Base(absPath))
				}
				if err := startWithWorkspace(absPath); err != nil {
					return err
				}
				if absPath != "" {
					fmt.Printf("✓ Alcatraz running with %s\n\n", absPath)
				}
			}

			envArgs := config.CollectAPIEnvArgs()
			c := compose.ExecInteractive("alcatraz", workdir, envArgs...)

			// Replace this process with `docker compose exec -it` so the
			// interactive shell gets the host TTY directly. Keeping the Go
			// process in between (exec.Command + Run) can break raw input
			// mode for CLIs like opencode, where Enter stops working.
			dockerPath, err := exec.LookPath(c.Path)
			if err != nil {
				return fmt.Errorf("docker not found: %v", err)
			}
			if err := syscall.Exec(dockerPath, c.Args, c.Env); err != nil {
				return fmt.Errorf("failed to exec docker: %v", err)
			}
			return nil
		},
	}
}

// shellRunCmd opens an interactive shell that first runs the given command
// (e.g. a session-resume command) and then drops into normal bash. Used by the
// TUI Sessions screen via the post-TUI next-action; hidden from normal help.
func shellRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "shell-run [command...]",
		Short:  "Open a shell that runs a command first, then stays interactive",
		Hidden: true,
		Args:   cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runCmd := strings.Join(args, " ")
			absPath := state.GetWorkspace()
			workdir := "/workspace"
			if absPath != "" {
				workdir = "/workspace/projects/" + filepath.Base(absPath)
			}
			if running := compose.IsRunning("alcatraz"); !running || !workspaceMounted(workdir) {
				if err := startWithWorkspace(absPath); err != nil {
					return err
				}
			}
			envArgs := config.CollectAPIEnvArgs()
			c := compose.ExecInteractiveRun("alcatraz", workdir, runCmd, envArgs...)
			dockerPath, err := exec.LookPath(c.Path)
			if err != nil {
				return fmt.Errorf("docker not found: %v", err)
			}
			if err := syscall.Exec(dockerPath, c.Args, c.Env); err != nil {
				return fmt.Errorf("failed to exec docker: %v", err)
			}
			return nil
		},
	}
}

func stopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop all containers",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := compose.Down(false)
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				return err
			}
			fmt.Println("✓ Containers stopped")
			return nil
		},
	}
}

func cleanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clean",
		Short: "Stop and remove everything including volumes",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := compose.Down(true)
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				return err
			}
			fmt.Println("✓ Cleanup complete")
			return nil
		},
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show container status",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := compose.Ps()
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			c.Run()

			ws := state.GetWorkspace()
			fmt.Printf("\nWorkspace: %s\n", ws)
			fmt.Printf("Mount:     %s -> /workspace\n", ws)
			return nil
		},
	}
}

func statsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Token usage/cost report metered by the Guard (per day/model)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !compose.IsRunning("guard") {
				return fmt.Errorf("Guard is not running. Start the stack first: alcatraz run")
			}
			// Aggregation runs inside the Guard container, where the stats
			// JSONL lives (audit volume). Same binary, -stats mode.
			c := compose.ExecRaw("guard", "/alcatraz", "-stats")
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	}
}

// The checkpoint logic lives in alcatraz.sh (pure host-side git); the CLI
// delegates to it the same way test-guard/test-security delegate to their
// scripts.
func shellPassthrough(use, short, action string, maxArgs int) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.MaximumNArgs(maxArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := exec.Command("bash", append([]string{filepath.Join(projectRoot, "alcatraz.sh"), action}, args...)...)
			c.Dir = projectRoot
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	}
}

func sessionsCmd() *cobra.Command {
	return shellPassthrough("sessions",
		"List resumable AI sessions per model (claude --continue etc.)", "sessions", 0)
}

func checkpointCmd() *cobra.Command {
	return shellPassthrough("checkpoint [message]",
		"Snapshot the workspace to a shadow git ref (auto on run/exec)", "checkpoint", 1)
}

func checkpointsCmd() *cobra.Command {
	return shellPassthrough("checkpoints",
		"List workspace checkpoints (1 = most recent)", "checkpoints", 0)
}

func rollbackCmd() *cobra.Command {
	return shellPassthrough("rollback [N|HASH]",
		"Restore workspace files to a checkpoint (default: latest)", "rollback", 1)
}

func resourcesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resources",
		Short: "Show live resource usage",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := compose.Ps()
			out, _ := c.Output()
			lines := strings.Split(string(out), "\n")
			var id string
			for _, line := range lines {
				if strings.Contains(line, "alcatraz") && !strings.Contains(line, "backend") && !strings.Contains(line, "proxy") {
					fields := strings.Fields(line)
					if len(fields) > 0 {
						id = fields[0]
						break
					}
				}
			}
			if id == "" {
				fmt.Println("Container not running")
				return nil
			}
			stats := exec.Command("docker", "stats", "--no-stream", id)
			stats.Stdout = os.Stdout
			stats.Stderr = os.Stderr
			return stats.Run()
		},
	}
}

func logsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logs [SERVICE]",
		Short: "Tail logs (default: alcatraz)",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := "alcatraz"
			if len(args) > 0 {
				switch args[0] {
				case "guard", "backend", "audit":
					svc = "guard"
				case "squid", "proxy":
					svc = "lighthouse"
				default:
					svc = args[0]
				}
			}
			c := compose.Logs(svc, true, 200)
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			fmt.Printf("Tailing logs for '%s' (Ctrl+C to exit)...\n\n", svc)
			return c.Run()
		},
	}
}

func testGuardCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test-guard",
		Short: "Run Guard tests",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := exec.Command("bash", filepath.Join(projectRoot, "test-guard.sh"))
			c.Dir = projectRoot
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	}
}

func testSecurityCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test-security",
		Short: "Run security isolation tests",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := exec.Command("bash", filepath.Join(projectRoot, "test-security.sh"))
			c.Dir = projectRoot
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	}
}
