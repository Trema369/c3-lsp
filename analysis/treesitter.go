package analysis

import (
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	"test/lsp/lsp"
)

// CompletionContext describes what kind of completion is needed
type CompletionContext int

const (
	CtxUnknown     CompletionContext = iota
	CtxModuleScope                   // after "module_name::"
	CtxDotAccess                     // after "variable."
	CtxImport                        // inside "import ..."
	CtxTopLevel                      // top level, suggest keywords + local symbols
)

// GetCompletionContext analyzes the cursor position and returns what kind
// of completion is needed and any relevant prefix (e.g. module name)
func GetCompletionContext(doc *Document, pos lsp.Position, src []byte) (CompletionContext, string) {
	if doc.Tree == nil {
		return CtxTopLevel, ""
	}

	lines := strings.Split(string(src), "\n")
	if pos.Line >= len(lines) {
		return CtxTopLevel, ""
	}

	line := lines[pos.Line]
	col := pos.Character
	if col > len(line) {
		col = len(line)
	}
	prefix := line[:col]

	// Check for module scope access: "something::"
	if strings.Contains(prefix, "::") {
		parts := strings.Split(prefix, "::")
		// get everything before the last "::"
		modulePart := strings.Join(parts[:len(parts)-1], "::")
		// extract just the last identifier before "::"
		words := strings.Fields(modulePart)
		if len(words) > 0 {
			module := words[len(words)-1]
			// strip any non-ident chars from start
			for i, c := range module {
				if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
					module = module[i:]
					break
				}
			}
			return CtxModuleScope, module
		}
	}

	// Check for dot access: "variable."
	trimmed := strings.TrimRight(prefix, " \t")
	if strings.HasSuffix(trimmed, ".") {
		// get the identifier before the dot
		before := trimmed[:len(trimmed)-1]
		words := strings.Fields(before)
		if len(words) > 0 {
			return CtxDotAccess, words[len(words)-1]
		}
	}

	// Check for import statement
	trimmedLine := strings.TrimSpace(prefix)
	if strings.HasPrefix(trimmedLine, "import ") {
		return CtxImport, ""
	}

	return CtxTopLevel, ""
}

// NodeAtPosition returns the deepest tree-sitter node at the given position
func NodeAtPosition(tree *sitter.Tree, pos lsp.Position) *sitter.Node {
	root := tree.RootNode()
	point := sitter.Point{
		Row:    uint(pos.Line),
		Column: uint(pos.Character),
	}
	return root.DescendantForPointRange(point, point)
}

// inferTypeFromTree uses the AST to find the declared type of a variable
func inferTypeFromTree(doc *Document, varName string) string {
	if doc.Tree == nil {
		return ""
	}
	src := []byte(doc.Text)
	return searchForVarType(doc.Tree.RootNode(), src, varName)
}

func searchForVarType(node *sitter.Node, src []byte, varName string) string {
	if node.Kind() == "declaration" {
		// declaration -> type, ident
		var typeName, name string
		for i := range node.ChildCount() {
			child := node.Child(i)
			if child == nil {
				continue
			}
			switch child.Kind() {
			case "type":
				// get the innermost type_ident or base_type_name
				typeName = extractTypeName(child, src)
			case "ident":
				name = string(src[child.StartByte():child.EndByte()])
			}
		}
		if name == varName && typeName != "" {
			return typeName
		}
	}

	for i := range node.ChildCount() {
		child := node.Child(i)
		if child != nil {
			if result := searchForVarType(child, src, varName); result != "" {
				return result
			}
		}
	}
	return ""
}

func extractTypeName(typeNode *sitter.Node, src []byte) string {
	// type -> path_type_ident -> type_ident
	// type -> base_type_name -> int/float/etc
	for i := range typeNode.ChildCount() {
		child := typeNode.Child(i)
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "path_type_ident":
			ti := findChildNode(child, "type_ident")
			if ti != nil {
				return string(src[ti.StartByte():ti.EndByte()])
			}
		case "base_type_name":
			if child.ChildCount() > 0 {
				inner := child.Child(0)
				if inner != nil {
					return string(src[inner.StartByte():inner.EndByte()])
				}
			}
		case "type_ident":
			return string(src[child.StartByte():child.EndByte()])
		}
	}
	return ""
}
