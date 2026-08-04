// Package guard manages the Guard user rules file
// (~/.alcatraz/guard-rules.yml) from the CLI/TUI side.
//
// The file is the single source of truth shared with the backend, which mounts
// the directory read-only and hot-reloads it. This package only reads and edits
// the YAML — the actual sanitization engine lives in the backend and is reached
// via `guard test` (docker exec … /alcatraz -check), so the CLI never
// duplicates the redaction logic.
package guard

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Dir returns the host directory mounted into the backend (~/.alcatraz).
func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".alcatraz")
}

// Path returns the rules file path.
func Path() string {
	return filepath.Join(Dir(), "guard-rules.yml")
}

// EnsureDir creates ~/.alcatraz (owned by the user) if missing. Doing this from
// the CLI before `docker compose up` prevents Docker from creating a
// root-owned directory at the bind-mount source.
func EnsureDir() error {
	return os.MkdirAll(Dir(), 0o700)
}

// EnsureTemplate writes the commented template if the rules file does not yet
// exist. Returns whether it created the file.
func EnsureTemplate() (bool, error) {
	if err := EnsureDir(); err != nil {
		return false, err
	}
	if _, err := os.Stat(Path()); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if err := os.WriteFile(Path(), []byte(template), 0o600); err != nil {
		return false, err
	}
	return true, nil
}

// Rule mirrors one entry under `redact:`.
type Rule struct {
	Name    string `yaml:"name"`
	Literal string `yaml:"literal,omitempty"`
	Regex   string `yaml:"regex,omitempty"`
	Replace string `yaml:"replace,omitempty"`
}

// File is the parsed rules file, for reading/status.
type File struct {
	Redact  []Rule   `yaml:"redact"`
	Allow   []string `yaml:"allow"`
	Markers struct {
		Enabled bool `yaml:"enabled"`
	} `yaml:"markers"`
	Mode string `yaml:"mode"`
}

// Load reads and parses the rules file. A missing file yields an empty File.
func Load() (*File, error) {
	data, err := os.ReadFile(Path())
	if err != nil {
		if os.IsNotExist(err) {
			return &File{}, nil
		}
		return nil, err
	}
	var f File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("rules file is not valid YAML: %w", err)
	}
	return &f, nil
}

// AddRule appends a redact rule to the file (creating it from the template
// first if needed), preserving existing comments. Exactly one of literal/regex
// must be non-empty; a regex is validated before writing.
func AddRule(r Rule) error {
	if (r.Literal == "") == (r.Regex == "") {
		return fmt.Errorf("provide exactly one of --literal or --regex")
	}
	if r.Name == "" {
		return fmt.Errorf("--name is required")
	}
	if r.Regex != "" {
		if _, err := regexp.Compile(r.Regex); err != nil {
			return fmt.Errorf("invalid regex: %w", err)
		}
	}
	if _, err := EnsureTemplate(); err != nil {
		return err
	}

	data, err := os.ReadFile(Path())
	if err != nil {
		return err
	}

	// Reject duplicate names up front for a clear error.
	existing, err := Load()
	if err != nil {
		return err
	}
	for _, e := range existing.Redact {
		if e.Name == r.Name {
			return fmt.Errorf("a rule named %q already exists", r.Name)
		}
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("rules file is not valid YAML: %w", err)
	}
	root := documentRoot(&doc)
	if root == nil {
		// Empty file — start a fresh mapping.
		root = &yaml.Node{Kind: yaml.MappingNode}
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
	}

	if idx := mappingValueIndex(root, "redact"); idx < 0 {
		// No redact key at all — append one with a fresh sequence.
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "redact"},
			&yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{ruleNode(r)}},
		)
	} else {
		seq := root.Content[idx]
		if seq.Kind != yaml.SequenceNode {
			// The key exists but its value is null/empty (e.g. the template,
			// which lists only commented examples). Replace it with a real
			// sequence, carrying over any attached comments.
			seq = &yaml.Node{
				Kind:        yaml.SequenceNode,
				HeadComment: seq.HeadComment,
				LineComment: seq.LineComment,
				FootComment: seq.FootComment,
			}
			root.Content[idx] = seq
		}
		seq.Content = append(seq.Content, ruleNode(r))
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return err
	}
	enc.Close()
	return os.WriteFile(Path(), buf.Bytes(), 0o600)
}

// documentRoot returns the mapping node at the root of a parsed document.
func documentRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		if doc.Content[0].Kind == yaml.MappingNode {
			return doc.Content[0]
		}
	}
	return nil
}

// mappingValueIndex returns the index in m.Content of the value node for key,
// or -1 if absent.
func mappingValueIndex(m *yaml.Node, key string) int {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i + 1
		}
	}
	return -1
}

func ruleNode(r Rule) *yaml.Node {
	kv := func(k, v string) []*yaml.Node {
		return []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: k},
			{Kind: yaml.ScalarNode, Value: v},
		}
	}
	m := &yaml.Node{Kind: yaml.MappingNode}
	m.Content = append(m.Content, kv("name", r.Name)...)
	if r.Literal != "" {
		m.Content = append(m.Content, kv("literal", r.Literal)...)
	} else {
		m.Content = append(m.Content, kv("regex", r.Regex)...)
	}
	if r.Replace != "" {
		m.Content = append(m.Content, kv("replace", r.Replace)...)
	}
	return m
}

// DeleteRule removes the named redact rule, preserving other content and
// comments. Returns an error if the rule is not found.
func DeleteRule(name string) error {
	data, err := os.ReadFile(Path())
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("rules file is not valid YAML: %w", err)
	}
	root := documentRoot(&doc)
	if root == nil {
		return fmt.Errorf("no rules to delete")
	}
	idx := mappingValueIndex(root, "redact")
	if idx < 0 || root.Content[idx].Kind != yaml.SequenceNode {
		return fmt.Errorf("rule %q not found", name)
	}
	seq := root.Content[idx]
	found := false
	kept := seq.Content[:0]
	for _, item := range seq.Content {
		if item.Kind == yaml.MappingNode && mappingScalar(item, "name") == name {
			found = true
			continue
		}
		kept = append(kept, item)
	}
	if !found {
		return fmt.Errorf("rule %q not found", name)
	}
	seq.Content = kept

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return err
	}
	enc.Close()
	return os.WriteFile(Path(), buf.Bytes(), 0o600)
}

// SetMode sets the `mode:` key to "balanced" or "strict", creating the file
// from the template first if needed and preserving all comments. The backend
// hot-reloads the change within ~1s.
func SetMode(mode string) error {
	if mode != "balanced" && mode != "strict" {
		return fmt.Errorf("mode must be 'balanced' or 'strict', got %q", mode)
	}
	if _, err := EnsureTemplate(); err != nil {
		return err
	}
	data, err := os.ReadFile(Path())
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("rules file is not valid YAML: %w", err)
	}
	root := documentRoot(&doc)
	if root == nil {
		root = &yaml.Node{Kind: yaml.MappingNode}
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
	}
	if idx := mappingValueIndex(root, "mode"); idx >= 0 {
		root.Content[idx].Value = mode
		root.Content[idx].Tag = "!!str"
	} else {
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "mode"},
			&yaml.Node{Kind: yaml.ScalarNode, Value: mode},
		)
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return err
	}
	enc.Close()
	return os.WriteFile(Path(), buf.Bytes(), 0o600)
}

// mappingScalar returns the scalar value for key in a mapping node.
func mappingScalar(m *yaml.Node, key string) string {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1].Value
		}
	}
	return ""
}

// Mask hides most of a sensitive value for display — terminals leak too.
func Mask(v string) string {
	v = strings.TrimSpace(v)
	if len(v) <= 4 {
		return strings.Repeat("•", len(v))
	}
	keep := 2
	return v[:keep] + strings.Repeat("•", len(v)-2*keep) + v[len(v)-keep:]
}
