package analysis

import (
	"os"
	"path/filepath"

	sitter "github.com/tree-sitter/go-tree-sitter"
	"test/lsp/lsp"
	"test/lsp/parser"
)

// StructInfo stores struct field information
type StructInfo struct {
	Fields []StructField
}

type StructField struct {
	Name string
	Type string
}

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
		indexFile(tree, src, s.GlobalIndex, s.StructIndex)
		s.mu.Unlock()

		tree.Close()
		return nil
	})
}

func indexFile(tree *sitter.Tree, src []byte, index map[string][]Symbol, structs map[string]StructInfo) {
	root := tree.RootNode()
	currentModule := "global"

	for i := range root.ChildCount() {
		child := root.Child(i)
		if child == nil {
			continue
		}

		switch child.Kind() {
		case "module_declaration":
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
				// index struct fields
				fields := extractStructFields(child, src)
				structs[name] = StructInfo{Fields: fields}
				structs[currentModule+"::"+name] = StructInfo{Fields: fields}
			}

		case "global_declaration":
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

func extractStructFields(structNode *sitter.Node, src []byte) []StructField {
	var fields []StructField
	body := findChild(structNode, "struct_body")
	if body == nil {
		return fields
	}
	for i := range body.ChildCount() {
		member := body.Child(i)
		if member == nil || member.Kind() != "struct_member_declaration" {
			continue
		}
		// get field name from identifier_list -> ident
		idList := findChild(member, "identifier_list")
		typeNode := findChild(member, "type")
		if idList == nil {
			continue
		}
		fieldName := ""
		nameNode := findChild(idList, "ident")
		if nameNode != nil {
			fieldName = string(src[nameNode.StartByte():nameNode.EndByte()])
		}
		fieldType := ""
		if typeNode != nil {
			fieldType = string(src[typeNode.StartByte():typeNode.EndByte()])
		}
		if fieldName != "" {
			fields = append(fields, StructField{Name: fieldName, Type: fieldType})
		}
	}
	return fields
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
