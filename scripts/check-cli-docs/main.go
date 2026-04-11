package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

var (
	docFiles = []string{
		"README.md",
		"CLI-GUIDE.md",
		"docs/reference/cli-reference.md",
		"docs/reference/commands.md",
		"docs/reference/cli-contract.md",
	}

	linkPattern    = regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)
	headingPattern = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*$`)
)

type anchorSet map[string]struct{}

func main() {
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve cwd: %v\n", err)
		os.Exit(1)
	}

	failures := make([]string, 0)
	anchorCache := make(map[string]anchorSet)

	for _, rel := range docFiles {
		if err := ensureExists(rel); err != nil {
			failures = append(failures, err.Error())
			continue
		}

		content, err := os.ReadFile(rel)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: read: %v", rel, err))
			continue
		}

		failures = append(failures, checkStructure(rel, string(content))...)
		failures = append(failures, checkLinks(root, rel, string(content), anchorCache)...)
	}

	failures = append(failures, checkTopLevelInventory()...)

	if len(failures) > 0 {
		for _, failure := range failures {
			fmt.Fprintf(os.Stderr, "doc-lint: %s\n", failure)
		}
		os.Exit(1)
	}

	fmt.Println("cli docs check passed")
}

func ensureExists(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("%s: missing: %v", path, err)
	}
	return nil
}

func checkStructure(path, content string) []string {
	var failures []string

	switch path {
	case "README.md":
		scanner := bufio.NewScanner(strings.NewReader(content))
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "| `cub-scout ") {
				failures = append(failures, fmt.Sprintf("%s:%d: README must not contain command-detail tables", path, lineNo))
			}
		}
	case "CLI-GUIDE.md":
		required := []string{
			"docs/reference/cli-reference.md",
			"docs/reference/commands.md",
			"docs/reference/cli-contract.md",
		}
		for _, needle := range required {
			if !strings.Contains(content, needle) {
				failures = append(failures, fmt.Sprintf("%s: missing required canonical doc link %q", path, needle))
			}
		}
		if regexp.MustCompile(`(?m)^##\s+`+"`").FindStringIndex(content) != nil {
			failures = append(failures, fmt.Sprintf("%s: command-level headings belong in docs/reference/commands.md, not CLI-GUIDE.md", path))
		}
	}

	return failures
}

func checkLinks(root, rel, content string, cache map[string]anchorSet) []string {
	var failures []string
	dir := filepath.Dir(rel)

	for _, match := range linkPattern.FindAllStringSubmatch(content, -1) {
		target := strings.TrimSpace(match[1])
		if target == "" || isExternal(target) {
			continue
		}

		targetPath := target
		anchor := ""
		if hash := strings.Index(target, "#"); hash >= 0 {
			targetPath = target[:hash]
			anchor = target[hash+1:]
		}

		resolved := rel
		if targetPath != "" {
			resolved = filepath.Clean(filepath.Join(dir, targetPath))
		}

		abs := filepath.Join(root, resolved)
		info, err := os.Stat(abs)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: broken link target %q", rel, target))
			continue
		}

		if anchor == "" || info.IsDir() || !strings.HasSuffix(resolved, ".md") {
			continue
		}

		anchors, err := anchorsForFile(resolved, cache)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", rel, err))
			continue
		}
		if _, ok := anchors[anchor]; !ok {
			failures = append(failures, fmt.Sprintf("%s: missing anchor %q in %s", rel, anchor, resolved))
		}
	}

	return failures
}

func checkTopLevelInventory() []string {
	helpOut, err := exec.Command("go", "run", "./cmd/cub-scout", "--help").CombinedOutput()
	if err != nil {
		return []string{fmt.Sprintf("command inventory: run cub-scout --help: %v", err)}
	}

	runtimeCommands := parseTopLevelCommandsFromHelp(string(helpOut))
	docCommands, err := parseTopLevelCommandsFromReference("docs/reference/cli-reference.md")
	if err != nil {
		return []string{err.Error()}
	}

	var failures []string
	for _, cmd := range runtimeCommands {
		if !slices.Contains(docCommands, cmd) {
			failures = append(failures, fmt.Sprintf("docs/reference/cli-reference.md: missing top-level command %q", cmd))
		}
	}
	for _, cmd := range docCommands {
		if !slices.Contains(runtimeCommands, cmd) {
			failures = append(failures, fmt.Sprintf("docs/reference/cli-reference.md: extra top-level command %q not present in cub-scout --help", cmd))
		}
	}
	return failures
}

func anchorsForFile(path string, cache map[string]anchorSet) (anchorSet, error) {
	if anchors, ok := cache[path]; ok {
		return anchors, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%s: open: %w", path, err)
	}
	defer f.Close()

	anchors := make(anchorSet)
	counts := make(map[string]int)
	scanner := bufio.NewScanner(f)
	inFence := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}

		if m := headingPattern.FindStringSubmatch(line); m != nil {
			slug := slugify(m[2])
			if slug == "" {
				continue
			}
			if n := counts[slug]; n > 0 {
				slug = fmt.Sprintf("%s-%d", slug, n)
			}
			counts[slugify(m[2])]++
			anchors[slug] = struct{}{}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%s: scan headings: %w", path, err)
	}

	cache[path] = anchors
	return anchors, nil
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case unicode.IsSpace(r) || r == '-':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func isExternal(target string) bool {
	return strings.HasPrefix(target, "http://") ||
		strings.HasPrefix(target, "https://") ||
		strings.HasPrefix(target, "mailto:") ||
		strings.HasPrefix(target, "app://")
}

func parseTopLevelCommandsFromHelp(help string) []string {
	var commands []string
	scanner := bufio.NewScanner(strings.NewReader(help))
	inCommands := false
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "Available Commands:"):
			inCommands = true
		case inCommands && strings.TrimSpace(line) == "":
			inCommands = false
		case inCommands:
			fields := strings.Fields(line)
			if len(fields) > 0 {
				commands = append(commands, fields[0])
			}
		}
	}
	return commands
}

func parseTopLevelCommandsFromReference(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%s: open: %w", path, err)
	}
	defer f.Close()

	var commands []string
	scanner := bufio.NewScanner(f)
	inTable := false
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "## Top-Level Commands"):
			inTable = true
		case inTable && strings.HasPrefix(line, "## "):
			inTable = false
		case inTable && strings.HasPrefix(strings.TrimSpace(line), "| `"):
			parts := strings.Split(line, "|")
			if len(parts) < 3 {
				continue
			}
			cmd := strings.TrimSpace(parts[1])
			cmd = strings.Trim(cmd, "`")
			if cmd != "" {
				commands = append(commands, cmd)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%s: scan: %w", path, err)
	}
	return commands, nil
}
