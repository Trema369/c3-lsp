package parser

import (
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestParse(t *testing.T) {
	lang := GetLanguage()
	if lang == nil {
		t.Fatal("language is nil")
	}

	p := sitter.NewParser()
	defer p.Close()
	p.SetLanguage(lang)

	src := []byte(`module test;
fn void main() {
}
`)
	tree := p.Parse(src, nil)
	defer tree.Close()

	root := tree.RootNode()
	t.Logf("root type: %s", root.Kind())
	t.Logf("child count: %d", root.ChildCount())
}
