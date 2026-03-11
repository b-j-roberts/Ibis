package cli

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// prompter provides interactive prompt helpers for CLI commands.
type prompter struct {
	reader *bufio.Reader
	out    io.Writer
}

func newPrompter(in io.Reader, out io.Writer) *prompter {
	return &prompter{
		reader: bufio.NewReader(in),
		out:    out,
	}
}

// input prompts for a text value with an optional default.
func (p *prompter) input(label, defaultVal string) (string, error) {
	if defaultVal != "" {
		fmt.Fprintf(p.out, "%s [%s]: ", label, defaultVal)
	} else {
		fmt.Fprintf(p.out, "%s: ", label)
	}
	line, err := p.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultVal, nil
	}
	return line, nil
}

// selectOne prompts the user to select one option from a numbered list.
// Returns the zero-based index of the selected option.
func (p *prompter) selectOne(label string, options []string, defaultIdx int) (int, error) {
	fmt.Fprintf(p.out, "\n%s:\n", label)
	for i, opt := range options {
		marker := "  "
		if i == defaultIdx {
			marker = "* "
		}
		fmt.Fprintf(p.out, "  %s%d) %s\n", marker, i+1, opt)
	}

	defaultStr := ""
	if defaultIdx >= 0 && defaultIdx < len(options) {
		defaultStr = strconv.Itoa(defaultIdx + 1)
	}

	val, err := p.input("Choice", defaultStr)
	if err != nil {
		return defaultIdx, err
	}

	idx, err := strconv.Atoi(val)
	if err != nil || idx < 1 || idx > len(options) {
		fmt.Fprintf(p.out, "Invalid choice, using default: %s\n", options[defaultIdx])
		return defaultIdx, nil
	}
	return idx - 1, nil
}

// selectMulti prompts the user to select multiple options from a numbered list.
// The user enters comma-separated numbers, or "*" for all.
// Returns the zero-based indices of selected options.
func (p *prompter) selectMulti(label string, options []string) ([]int, error) {
	fmt.Fprintf(p.out, "\n%s:\n", label)
	for i, opt := range options {
		fmt.Fprintf(p.out, "  %d) %s\n", i+1, opt)
	}

	val, err := p.input("Select (comma-separated, or * for all)", "*")
	if err != nil {
		return nil, err
	}

	if val == "*" {
		indices := make([]int, len(options))
		for i := range options {
			indices[i] = i
		}
		return indices, nil
	}

	parts := strings.Split(val, ",")
	var indices []int
	seen := make(map[int]bool)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		idx, err := strconv.Atoi(part)
		if err != nil || idx < 1 || idx > len(options) {
			fmt.Fprintf(p.out, "Skipping invalid choice: %s\n", part)
			continue
		}
		if !seen[idx-1] {
			indices = append(indices, idx-1)
			seen[idx-1] = true
		}
	}

	if len(indices) == 0 {
		// Default to all if nothing valid was selected.
		indices = make([]int, len(options))
		for i := range options {
			indices[i] = i
		}
		fmt.Fprintln(p.out, "No valid selections, defaulting to all.")
	}

	return indices, nil
}

// confirm prompts for a yes/no confirmation.
func (p *prompter) confirm(label string, defaultYes bool) (bool, error) {
	suffix := " [Y/n]: "
	if !defaultYes {
		suffix = " [y/N]: "
	}
	fmt.Fprintf(p.out, "%s%s", label, suffix)

	line, err := p.reader.ReadString('\n')
	if err != nil {
		return defaultYes, err
	}
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return defaultYes, nil
	}
	return line == "y" || line == "yes", nil
}
