package analysis

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	sitter "github.com/tree-sitter/go-tree-sitter"
	"test/lsp/lsp"
	"test/lsp/parser"
)

type Document struct {
	Text string
	Tree *sitter.Tree
}

type State struct {
	mu          sync.RWMutex
	Documents   map[string]*Document
	GlobalIndex map[string][]Symbol
	parser      *sitter.Parser
}

type Symbol struct {
	Name   string
	Kind   int
	Detail string
}

func NewState() *State {
	p := sitter.NewParser()
	p.SetLanguage(parser.GetLanguage())
	return &State{
		Documents:   make(map[string]*Document),
		GlobalIndex: make(map[string][]Symbol),
		parser:      p,
	}
}

type C3ProjectManifest struct {
	Target      string   `json:"target"`
	Dependecies []string `json:"dependencies"`
}

func (s *State) LoadProjectManifest(rootPath string) (*C3ProjectManifest, error) {
	manifestPath := filepath.Join(rootPath, "project.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var manifest C3ProjectManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func (s *State) OpenDocument(uri, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tree := s.parser.Parse([]byte(text), nil)
	s.Documents[uri] = &Document{Text: text, Tree: tree}
}

func (s *State) UpdateDocument(uri, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.Documents[uri]
	var oldTree *sitter.Tree
	if old != nil {
		oldTree = old.Tree
	}
	tree := s.parser.Parse([]byte(text), oldTree)
	s.Documents[uri] = &Document{Text: text, Tree: tree}
}

func (s *State) Analyze(uri string) ([]lsp.Diagnostic, error) {
	s.mu.RLock()
	doc, exists := s.Documents[uri]
	s.mu.RUnlock()
	if !exists {
		return []lsp.Diagnostic{}, nil
	}

	tmpFile, err := os.CreateTemp("", "c3lsp_*.c3")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(doc.Text); err != nil {
		return nil, err
	}

	cmd := exec.Command("c3c", "compile", "-C", "--lsp", tmpFile.Name())
	output, _ := cmd.CombinedOutput()
	return parseC3cOutput(string(output)), nil
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
func (s *State) GetGlobalIndexCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.GlobalIndex)
}
func (s *State) GetCompletionItems(uri string, pos lsp.Position) []lsp.CompletionItem {
	s.mu.RLock()
	doc, exists := s.Documents[uri]
	s.mu.RUnlock()
	if !exists {
		return nil
	}

	src := []byte(doc.Text)
	ctx, prefix := GetCompletionContext(doc, pos, src)

	switch ctx {
	case CtxModuleScope:
		return s.getModuleCompletions(doc, prefix)
	case CtxDotAccess:
		return s.getDotCompletions(doc, prefix)
	case CtxImport:
		return s.getImportCompletions()
	default:
		return s.getTopLevelCompletions(doc)
	}
}

func (s *State) getModuleCompletions(doc *Document, moduleName string) []lsp.CompletionItem {
	// resolve short name to full module path using imports
	fullModule := resolveModuleName(doc.Text, moduleName)
	if fullModule == "" {
		fullModule = moduleName
	}

	var items []lsp.CompletionItem
	// direct match
	if symbols, ok := s.GlobalIndex[fullModule]; ok {
		for _, sym := range symbols {
			items = append(items, lsp.CompletionItem{
				Label:  sym.Name,
				Kind:   sym.Kind,
				Detail: sym.Detail,
			})
		}
		return items
	}

	// fuzzy match — check if any module ends with this name
	for mod, symbols := range s.GlobalIndex {
		parts := strings.Split(mod, "::")
		if parts[len(parts)-1] == moduleName {
			for _, sym := range symbols {
				items = append(items, lsp.CompletionItem{
					Label:  sym.Name,
					Kind:   sym.Kind,
					Detail: sym.Detail,
				})
			}
		}
	}
	return items
}

func (s *State) getDotCompletions(doc *Document, varName string) []lsp.CompletionItem {
	// find the type of varName in the document
	typeName := inferType(doc.Text, varName)
	if typeName == "" {
		return nil
	}

	var items []lsp.CompletionItem
	for _, symbols := range s.GlobalIndex {
		for _, sym := range symbols {
			if sym.Detail == typeName {
				items = append(items, lsp.CompletionItem{
					Label: sym.Name,
					Kind:  sym.Kind,
				})
			}
		}
	}
	return items
}

func (s *State) getImportCompletions() []lsp.CompletionItem {
	seen := make(map[string]bool)
	var items []lsp.CompletionItem
	for mod := range s.GlobalIndex {
		parts := strings.Split(mod, "::")
		// suggest top-level module names like "std"
		top := parts[0]
		if !seen[top] {
			seen[top] = true
			items = append(items, lsp.CompletionItem{
				Label: top,
				Kind:  lsp.ModuleCompletion,
			})
		}
	}
	return items
}

func (s *State) getTopLevelCompletions(doc *Document) []lsp.CompletionItem {
	items := []lsp.CompletionItem{
		{Label: "module", Kind: lsp.KeywordCompletion, Detail: "Declare module"},
		{Label: "import", Kind: lsp.KeywordCompletion, Detail: "Import module"},
		{Label: "fn", Kind: lsp.KeywordCompletion, Detail: "Function definition"},
		{Label: "struct", Kind: lsp.KeywordCompletion, Detail: "Struct definition"},
		{Label: "enum", Kind: lsp.KeywordCompletion, Detail: "Enum definition"},
		{Label: "defer", Kind: lsp.KeywordCompletion, Detail: "Defer execution"},
		{Label: "return", Kind: lsp.KeywordCompletion, Detail: "Return value"},
		{Label: "if", Kind: lsp.KeywordCompletion, Detail: "Conditional"},
		{Label: "for", Kind: lsp.KeywordCompletion, Detail: "Loop"},
		{Label: "while", Kind: lsp.KeywordCompletion, Detail: "While loop"},
		{Label: "void", Kind: lsp.KeywordCompletion, Detail: "Void type"},
		{Label: "int", Kind: lsp.KeywordCompletion, Detail: "Integer type"},
		{Label: "String", Kind: lsp.KeywordCompletion, Detail: "String type"},
		{Label: "bool", Kind: lsp.KeywordCompletion, Detail: "Boolean type"},
	}

	// add local functions from document
	items = append(items, extractLocalSymbols(doc)...)

	// add imported module short names
	items = append(items, extractImportedModules(doc.Text)...)

	return items
}

// resolveModuleName finds the full module path for a short name from imports
func resolveModuleName(text, shortName string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "import ") {
			continue
		}
		imp := strings.TrimPrefix(line, "import ")
		imp = strings.TrimSuffix(imp, ";")
		// handle comma separated: "import std::io, std::math"
		for _, mod := range strings.Split(imp, ",") {
			mod = strings.TrimSpace(mod)
			parts := strings.Split(mod, "::")
			if parts[len(parts)-1] == shortName {
				return mod
			}
		}
	}
	return ""
}

// inferType tries to find the declared type of a variable in the document text
func inferType(text, varName string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		// look for patterns like "TypeName varName" or "TypeName varName ="
		if strings.Contains(line, varName) {
			fields := strings.Fields(line)
			for i, f := range fields {
				if f == varName && i > 0 {
					return fields[i-1]
				}
			}
		}
	}
	return ""
}

// extractLocalSymbols pulls functions and structs from the document tree
func extractLocalSymbols(doc *Document) []lsp.CompletionItem {
	var items []lsp.CompletionItem
	if doc.Tree == nil {
		return items
	}
	root := doc.Tree.RootNode()
	src := []byte(doc.Text)
	walkNode(root, src, &items)
	return items
}

func walkNode(node *sitter.Node, src []byte, items *[]lsp.CompletionItem) {
	switch node.Kind() {
	case "func_definition":
		header := findChildNode(node, "func_header")
		if header != nil {
			nameNode := findChildNode(header, "ident")
			if nameNode != nil {
				name := string(src[nameNode.StartByte():nameNode.EndByte()])
				*items = append(*items, lsp.CompletionItem{
					Label: name,
					Kind:  lsp.FunctionCompletion,
				})
			}
		}
	case "struct_declaration":
		nameNode := findChildNode(node, "type_ident")
		if nameNode != nil {
			name := string(src[nameNode.StartByte():nameNode.EndByte()])
			*items = append(*items, lsp.CompletionItem{
				Label: name,
				Kind:  lsp.StructCompletion,
			})
		}
	}
	for i := range node.ChildCount() {
		child := node.Child(i)
		if child != nil {
			walkNode(child, src, items)
		}
	}
}

func findChildNode(node *sitter.Node, kind string) *sitter.Node {
	for i := range node.ChildCount() {
		child := node.Child(i)
		if child != nil && child.Kind() == kind {
			return child
		}
	}
	return nil
}

func extractImportedModules(text string) []lsp.CompletionItem {
	var items []lsp.CompletionItem
	seen := make(map[string]bool)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "import ") {
			continue
		}
		imp := strings.TrimPrefix(line, "import ")
		imp = strings.TrimSuffix(imp, ";")
		for _, mod := range strings.Split(imp, ",") {
			mod = strings.TrimSpace(mod)
			parts := strings.Split(mod, "::")
			shortName := parts[len(parts)-1]
			if !seen[shortName] {
				seen[shortName] = true
				items = append(items, lsp.CompletionItem{
					Label:  shortName,
					Kind:   lsp.ModuleCompletion,
					Detail: "module " + mod,
				})
			}
		}
	}
	return items
}
