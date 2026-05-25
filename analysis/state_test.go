package analysis

import (
	"testing"
)

func TestParseC3cOutput(t *testing.T) {
	// 1. Arrange: Define a mock block of raw text that mimics c3c compile -C --lsp exactly
	mockCompilerOutput := `> BEGINLSP
> LSPERR|error|"/data/data/com.termux/files/home/projects/c3-lsp/test.c3"|4|13|"You cannot cast 'String' to 'int'."
> LSPERR|warning|"/data/data/com.termux/files/home/projects/c3-lsp/test.c3"|10|5|"Unused variable 'foo'."
> ENDLSP-ERROR
Some trailing compiler debug garbage text`

	// 2. Act: Run our string splitting parser against the fake text chunk
	result := parseC3cOutput(mockCompilerOutput)

	// 3. Assert: Verify the lengths match up first
	if len(result) != 2 {
		t.Fatalf("Expected exactly 2 diagnostics parsed, but got %d", len(result))
	}

	// Test Case A: The 'error' string parsing block
	errDiag := result[0]
	if errDiag.Severity != 1 {
		t.Errorf("Expected severity 1 (Error), got %d", errDiag.Severity)
	}
	// Verify coordinate translation (1-indexed '4:13' -> 0-indexed '3:12')
	if errDiag.Range.Start.Line != 3 || errDiag.Range.Start.Character != 12 {
		t.Errorf("Expected 0-indexed start position (3, 12), got (%d, %d)",
			errDiag.Range.Start.Line, errDiag.Range.Start.Character)
	}
	// Verify stripping of outer compiler text quotes
	expectedErrMsg := "You cannot cast 'String' to 'int'."
	if errDiag.Message != expectedErrMsg {
		t.Errorf("Expected message %q, got %q", expectedErrMsg, errDiag.Message)
	}

	// Test Case B: The 'warning' string parsing block
	warnDiag := result[1]
	if warnDiag.Severity != 2 {
		t.Errorf("Expected severity 2 (Warning), got %d", warnDiag.Severity)
	}
	// Verify coordinate translation (1-indexed '10:5' -> 0-indexed '9:4')
	if warnDiag.Range.Start.Line != 9 || warnDiag.Range.Start.Character != 4 {
		t.Errorf("Expected 0-indexed start position (9, 4), got (%d, %d)",
			warnDiag.Range.Start.Line, warnDiag.Range.Start.Character)
	}
	expectedWarnMsg := "Unused variable 'foo'."
	if warnDiag.Message != expectedWarnMsg {
		t.Errorf("Expected message %q, got %q", expectedWarnMsg, warnDiag.Message)
	}
}

func TestParseC3cOutput_EmptyInput(t *testing.T) {
	// Ensure that clean compilation runs don't panic and return valid empty arrays
	result := parseC3cOutput("> BEGINLSP\n> ENDLSP-ERROR")
	if result == nil || len(result) != 0 {
		t.Errorf("Expected non-nil empty slice [], got %v", result)
	}
}
