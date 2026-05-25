package analysis

import (
	"os"
	"path/filepath"

	sitter "github.com/tree-sitter/go-tree-sitter"
	"test/lsp/lsp"
	"test/lsp/parser"
)

func (s *State) IndexDirectory(dirPath string) error {
	lang := parser.GetLanguage()
	p := sitter.NewParser()
	p.SetLanguage(lang)

	return filepath.WalkDir(dirPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".c3" {
			return nil
		}

		src, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		tree := p.Parse(src, nil)
		if tree == nil {
			return nil
		}

		s.mu.Lock()
		indexFile(tree, src, s.GlobalIndex)
		s.mu.Unlock()

		tree.Close()
		return nil
	})
}

func indexFile(tree *sitter.Tree, src []byte, index map[string][]Symbol) {
	root := tree.RootNode()
	currentModule := "global"

	for i := range root.ChildCount() {
		child := root.Child(i)
		if child == nil {
			continue
		}

		switch child.Kind() {
		case "module_declaration":
			// path_ident contains the full module path
			pathIdent := findChild(child, "path_ident")
			if pathIdent != nil {
				currentModule = string(src[pathIdent.StartByte():pathIdent.EndByte()])
			}

		case "func_definition":
			header := findChild(child, "func_header")
			if header != nil {
				nameNode := findChild(header, "ident")
				if nameNode != nil {
					name := string(src[nameNode.StartByte():nameNode.EndByte()])
					index[currentModule] = append(index[currentModule], Symbol{
						Name:   name,
						Kind:   lsp.FunctionCompletion,
						Detail: currentModule + "::" + name,
					})
				}
			}

		case "struct_declaration":
			nameNode := findChild(child, "type_ident")
			if nameNode != nil {
				name := string(src[nameNode.StartByte():nameNode.EndByte()])
				index[currentModule] = append(index[currentModule], Symbol{
					Name:   name,
					Kind:   lsp.StructCompletion,
					Detail: "struct " + name,
				})
			}

		case "global_declaration":
			// const_declaration is nested inside global_declaration
			constDecl := findChild(child, "const_declaration")
			if constDecl != nil {
				nameNode := findChild(constDecl, "const_ident")
				if nameNode != nil {
					name := string(src[nameNode.StartByte():nameNode.EndByte()])
					index[currentModule] = append(index[currentModule], Symbol{
						Name:   name,
						Kind:   lsp.ConstantCompletion,
						Detail: "const " + name,
					})
				}
			}
		}
	}
}

func findChild(node *sitter.Node, kind string) *sitter.Node {
	for i := range node.ChildCount() {
		child := node.Child(i)
		if child != nil && child.Kind() == kind {
			return child
		}
	}
	return nil
}
