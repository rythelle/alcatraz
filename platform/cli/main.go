package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alcatraz/alcatraz/cli/internal/config"
	"github.com/alcatraz/alcatraz/cli/internal/docker"
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
		stopCmd(),
		cleanCmd(),
		statusCmd(),
		resourcesCmd(),
		logsCmd(),
		testGuardianCmd(),
		testSecurityCmd(),
		tuiCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
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
			os.Setenv("ALCATRAZ_WORKSPACE", absPath)
			docker.EnsureContextDir(projectRoot)

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

			if path != "" {
				if _, err := os.Stat(path); err != nil {
					return fmt.Errorf("directory does not exist: %s", path)
				}
				absPath, _ := filepath.Abs(path)
				state.SetWorkspace(absPath)
				os.Setenv("ALCATRAZ_WORKSPACE", absPath)
				docker.EnsureContextDir(projectRoot)

				if compose.IsRunning("alcatraz") {
					compose.Down(false).Run()
				}

				imageExists := exec.Command("docker", "image", "inspect", "alcatraz:latest").Run() == nil
				var dcCmd *exec.Cmd
				if !imageExists {
					dcCmd = compose.Up(false, true)
				} else {
					dcCmd = compose.Up(true, false)
				}
				if err := dcCmd.Run(); err != nil {
					return err
				}
				fmt.Printf("✓ Alcatraz running with %s\n\n", absPath)
			}

			envArgs := config.CollectAPIEnvArgs()
			c := compose.ExecInteractive("alcatraz", "/workspace", envArgs...)
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				if _, ok := err.(*exec.ExitError); ok {
					return nil // shell exited normally (exit, Ctrl+D, Ctrl+C)
				}
				return err
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
				case "guardian", "backend", "audit":
					svc = "alcatraz-backend"
				case "squid", "proxy":
					svc = "proxy-whitelist"
				default:
					svc = args[0]
				}
			}
			c := compose.Logs(svc, true)
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			fmt.Printf("Tailing logs for '%s' (Ctrl+C to exit)...\n\n", svc)
			return c.Run()
		},
	}
}

func testGuardianCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test-guardian",
		Short: "Run Data Guardian tests",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := exec.Command("bash", filepath.Join(projectRoot, "test-guardian.sh"))
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
