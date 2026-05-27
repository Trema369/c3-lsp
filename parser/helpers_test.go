package parser

import (
	"fmt"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

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
