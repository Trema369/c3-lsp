package analysis

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"test/lsp/lsp"
)

type State struct {
	Documents map[string]string
}

func NewState() State {
	return State{Documents: map[string]string{}}
}

func (s *State) OpenDocument(document, text string) {
	s.Documents[document] = text
}

func (s *State) UpdateDocument(uri, text string) {
	s.Documents[uri] = text
}

func (s *State) Analyze(uri string) ([]lsp.Diagnostic, error) {
	text, exists := s.Documents[uri]
	if !exists {
		return []lsp.Diagnostic{}, nil
	}

	tmpFile, err := os.CreateTemp("", "c3lsp_*.c3")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(text); err != nil {
		return nil, err
	}

	cmd := exec.Command("c3c", "compile", "-C", "--lsp", tmpFile.Name())

	output, _ := cmd.CombinedOutput()

	diagnostics := parseC3cOutput(string(output))

	return diagnostics, nil
}

func parseC3cOutput(output string) []lsp.Diagnostic {
	diagnostics := []lsp.Diagnostic{}
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		line = strings.TrimPrefix(line, "> ")

		if !strings.HasPrefix(line, "LSPERR|") {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 6 {
			continue
		}

		severity := 1 // 1 = Error
		if parts[1] == "warning" {
			severity = 2 // 2 = Warning
		}

		lineNum, err1 := strconv.Atoi(parts[3])
		colNum, err2 := strconv.Atoi(parts[4])
		if err1 != nil || err2 != nil {
			continue
		}

		startLine := lineNum - 1
		startChar := colNum - 1

		message := strings.Trim(parts[5], "\"")

		diagnostics = append(diagnostics, lsp.Diagnostic{
			Range: lsp.Range{
				Start: lsp.Position{Line: startLine, Character: startChar},
				End:   lsp.Position{Line: startLine, Character: startChar + 3},
			},
			Severity: severity,
			Source:   "c3c",
			Message:  message,
		})
	}

	return diagnostics
}
