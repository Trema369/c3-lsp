package parser

import (
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestStructFields(t *testing.T) {
	src := []byte(`
module test;

struct Person {
    int age;
    String name;
    float height;
}

fn void main() {
    Person p;
    p.age = 10;
}
`)
	lang := GetLanguage()
	p := sitter.NewParser()
	p.SetLanguage(lang)
	tree := p.Parse(src, nil)
	defer tree.Close()

	root := tree.RootNode()
	printTree(root, src, 0)
}
