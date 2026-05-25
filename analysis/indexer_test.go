package analysis

import (
	"testing"
)

func TestIndexDirectory(t *testing.T) {
	state := NewState()
	err := state.IndexDirectory("/usr/local/lib/c3/std")
	if err != nil {
		t.Fatalf("IndexDirectory failed: %v", err)
	}

	t.Logf("Indexed %d modules", len(state.GlobalIndex))
	for mod, symbols := range state.GlobalIndex {
		t.Logf("Module: %s (%d symbols)", mod, len(symbols))
		for _, sym := range symbols {
			t.Logf("  %s (kind=%d)", sym.Name, sym.Kind)
		}
	}
}
