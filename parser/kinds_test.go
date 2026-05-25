package parser

import (
	"fmt"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestNodeKinds(t *testing.T) {
	src := []byte(`
module test;
import std::io;

struct Person {
    int age;
    String name;
}

fn void greet(Person p) {
    io::printn("hello");
}

const int MAX = 100;
`)
	lang := GetLanguage()
	p := sitter.NewParser()
	p.SetLanguage(lang)
	tree := p.Parse(src, nil)
	defer tree.Close()

	root := tree.RootNode()
	printTree(root, src, 0)
}

func printTree(node *sitter.Node, src []byte, depth int) {
	indent := ""
	for i := 0; i < depth; i++ {
		indent += "  "
	}
	text := ""
	if node.ChildCount() == 0 {
		text = fmt.Sprintf(" = %q", string(src[node.StartByte():node.EndByte()]))
	}
	fmt.Printf("%s[%s]%s\n", indent, node.Kind(), text)
	for i := range node.ChildCount() {
		child := node.Child(i)
		if child != nil {
			printTree(child, src, depth+1)
		}
	}
}
