package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/alcatraz/alcatraz/cli/internal/guard"
	"github.com/spf13/cobra"
)

func guardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "guard",
		Short: "Manage the Guard redaction rules",
		Long: `Manage ~/.alcatraz/guard-rules.yml — custom redactions, allowlist,
inline code markers and strict mode. The file is mounted read-only into the
backend and hot-reloaded on save.`,
	}
	cmd.AddCommand(
		guardAddCmd(),
		guardListCmd(),
		guardModeCmd(),
		guardTestCmd(),
		guardStatusCmd(),
		guardAuditCmd(),
	)
	return cmd
}

func guardModeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mode [balanced|strict]",
		Short: "Show or set the redaction mode",
		Long: `Show the current mode, or set it to balanced or strict.

  balanced  built-in patterns with checksum validation (default)
  strict    also redacts context-free look-alikes — bare SSNs, Mercosul
            plates, hyphenated CEPs — at the cost of more false positives.

With no argument, prints the current mode. The backend hot-reloads a change
within ~1s.`,
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: []string{"balanced", "strict"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				f, err := guard.Load()
				if err != nil {
					return err
				}
				mode := f.Mode
				if mode == "" {
					mode = "balanced"
				}
				fmt.Printf("Mode: %s\n", mode)
				return nil
			}
			mode := strings.ToLower(args[0])
			if err := guard.SetMode(mode); err != nil {
				return err
			}
			fmt.Printf("✓ Mode set to %q. The backend will hot-reload it within ~1s.\n", mode)
			return nil
		},
	}
}

func guardAddCmd() *cobra.Command {
	var r guard.Rule
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a custom redaction rule",
		Example: `  alcatraz guard add --name formula --literal "k = 1.4423" --replace "[FORMULA]"
  alcatraz guard add --name acme --regex 'AcmeAlgo(V[0-9]+)?'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if created, err := guard.EnsureTemplate(); err != nil {
				return err
			} else if created {
				fmt.Printf("✓ Created %s\n", guard.Path())
			}
			if err := guard.AddRule(r); err != nil {
				return err
			}
			fmt.Printf("✓ Rule %q added. The backend will hot-reload it within ~1s.\n", r.Name)
			return nil
		},
	}
	cmd.Flags().StringVar(&r.Name, "name", "", "rule name (shown in audit log)")
	cmd.Flags().StringVar(&r.Literal, "literal", "", "exact substring to redact")
	cmd.Flags().StringVar(&r.Regex, "regex", "", "Go RE2 pattern to redact")
	cmd.Flags().StringVar(&r.Replace, "replace", "", "replacement text (default [REDACTED_BY_ALCATRAZ_CUSTOM])")
	return cmd
}

func guardListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List custom rules (values masked)",
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := guard.Load()
			if err != nil {
				return err
			}
			if len(f.Redact) == 0 && len(f.Allow) == 0 {
				fmt.Println("No custom rules yet.")
				fmt.Println("  Add one:  alcatraz guard add --name <n> --literal <value>")
				return nil
			}
			mode := f.Mode
			if mode == "" {
				mode = "balanced"
			}
			fmt.Printf("Rules file: %s\n", guard.Path())
			fmt.Printf("Mode: %s   Markers: %v\n\n", mode, f.Markers.Enabled)

			if len(f.Redact) > 0 {
				fmt.Println("🛡  Custom redactions:")
				for _, r := range f.Redact {
					kind, val := "literal", r.Literal
					if r.Regex != "" {
						kind, val = "regex", r.Regex
					}
					repl := r.Replace
					if repl == "" {
						repl = "[REDACTED_BY_ALCATRAZ_CUSTOM]"
					}
					fmt.Printf("  • %-20s %s=%s → %s\n", r.Name, kind, guard.Mask(val), repl)
				}
			}
			if len(f.Allow) > 0 {
				fmt.Printf("\n✅ Allowlist (%d entries, never redacted):\n", len(f.Allow))
				for _, a := range f.Allow {
					fmt.Printf("  • %s\n", guard.Mask(a))
				}
			}
			return nil
		},
	}
}

func guardTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test <text>",
		Short: "Run text through the live Guard engine and show the result",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !compose.IsRunning("guard") {
				return fmt.Errorf("backend is not running — start Alcatraz first (alcatraz run)")
			}
			text := strings.Join(args, " ")
			c := compose.ExecRaw("guard", "/alcatraz", "-check")
			c.Stdin = strings.NewReader(text)
			var out bytes.Buffer
			c.Stdout = &out
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				return err
			}
			result := out.String()
			fmt.Println("Input:")
			fmt.Printf("  %s\n\n", text)
			fmt.Println("Sanitized:")
			fmt.Printf("  %s\n", result)
			if result == text {
				fmt.Println("\n(no redactions — nothing matched)")
			}
			return nil
		},
	}
}

func guardStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show rule count, mode, and backend reload state",
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := guard.Load()
			if err != nil {
				fmt.Printf("⚠  Rules file problem: %v\n", err)
			} else {
				mode := f.Mode
				if mode == "" {
					mode = "balanced"
				}
				fmt.Printf("Rules file:   %s\n", guard.Path())
				fmt.Printf("Custom rules: %d\n", len(f.Redact))
				fmt.Printf("Allowlist:    %d\n", len(f.Allow))
				fmt.Printf("Markers:      %v\n", f.Markers.Enabled)
				fmt.Printf("Mode:         %s\n", mode)
			}

			fmt.Println()
			if !compose.IsRunning("guard") {
				fmt.Println("Backend:      not running")
				return nil
			}
			fmt.Println("Backend:      running")

			// Surface the most recent reload / error line from backend logs.
			logs := compose.Logs("guard", false, 500)
			outBytes, _ := logs.Output()
			var lastReload, lastError string
			sc := bufio.NewScanner(bytes.NewReader(outBytes))
			for sc.Scan() {
				line := sc.Text()
				switch {
				case strings.Contains(line, "rules_reload_failed"), strings.Contains(line, "initial load failed"):
					lastError = line
				case strings.Contains(line, "rules_reloaded"), strings.Contains(line, "guard rules loaded"):
					lastReload = line
				}
			}
			if lastReload != "" {
				fmt.Printf("Last reload:  %s\n", condense(lastReload))
			}
			if lastError != "" {
				fmt.Printf("⚠  Last error: %s\n", condense(lastError))
			}
			return nil
		},
	}
}

func guardAuditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "audit",
		Short: "Summarize the Guard audit log (what was redacted)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !compose.IsRunning("guard") {
				return fmt.Errorf("backend is not running — start Alcatraz first (alcatraz run)")
			}
			c := compose.ExecRaw("guard", "cat", "/var/log/alcatraz/audit.log")
			var out bytes.Buffer
			c.Stdout = &out
			if err := c.Run(); err != nil {
				return fmt.Errorf("read audit log: %w", err)
			}
			fmt.Println(guard.SummarizeAudit(out.Bytes()))
			return nil
		},
	}
}

// condense trims a zerolog line to its message and key fields for display.
func condense(line string) string {
	line = strings.TrimSpace(line)
	if i := strings.Index(line, "| "); i >= 0 && i+2 < len(line) {
		line = line[i+2:]
	}
	if len(line) > 120 {
		line = line[:120] + "…"
	}
	return line
}
