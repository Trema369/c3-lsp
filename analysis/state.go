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
	StructIndex map[string]StructInfo
	docModules  map[string][]string
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
		StructIndex: make(map[string]StructInfo),
		docModules:  make(map[string][]string),
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
	if tree != nil {
		s.reindexDocument(uri, tree, []byte(text))
	}
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
	if tree != nil {
		s.reindexDocument(uri, tree, []byte(text))
	}
}

func (s *State) IndexProject(projectRoot string) error {
	// always index all .c3 files in project root
	if err := s.IndexDirectory(projectRoot); err != nil {
		return err
	}

	// optionally read project.json for vendor dependencies
	manifestPath := filepath.Join(projectRoot, "project.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		// no project.json is fine, we already indexed the directory
		return nil
	}

	var manifest struct {
		DependencySearchPaths []string `json:"dependency-search-paths"`
		Dependencies          []string `json:"dependencies"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil
	}

	// index vendor dependencies
	for _, searchPath := range manifest.DependencySearchPaths {
		for _, dep := range manifest.Dependencies {
			depPath := filepath.Join(projectRoot, searchPath, dep)
			if info, err := os.Stat(depPath); err == nil && info.IsDir() {
				s.IndexDirectory(depPath)
			}
		}
	}

	return nil
}

func (s *State) reindexDocument(uri string, tree *sitter.Tree, src []byte) {
	// clear previously indexed symbols from this document
	if modules, ok := s.docModules[uri]; ok {
		for _, mod := range modules {
			delete(s.GlobalIndex, mod)
			delete(s.StructIndex, mod)
		}
	}

	// re-index into temporary maps then merge
	tmpIndex := make(map[string][]Symbol)
	tmpStructs := make(map[string]StructInfo)
	indexFile(tree, src, tmpIndex, tmpStructs)

	// track which modules this doc contributes
	var modules []string
	for mod, syms := range tmpIndex {
		s.GlobalIndex[mod] = append(s.GlobalIndex[mod], syms...)
		modules = append(modules, mod)
	}
	for name, info := range tmpStructs {
		s.StructIndex[name] = info
	}
	s.docModules[uri] = modules
}

func (s *State) Analyze(uri string, fullBuild bool) ([]lsp.Diagnostic, error) {
	s.mu.RLock()
	doc, exists := s.Documents[uri]
	s.mu.RUnlock()
	if !exists {
		return []lsp.Diagnostic{}, nil
	}

	path := strings.TrimPrefix(uri, "file://")
	root := FindProjectRoot(path)

	// write current file to temp so unsaved changes are analyzed
	tmpFile, err := os.CreateTemp("", "c3lsp_*.c3")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()
	if _, err := tmpFile.WriteString(doc.Text); err != nil {
		return nil, err
	}

	var cmd *exec.Cmd

	if fullBuild && root != "" {
		if _, err := os.Stat(filepath.Join(root, "project.json")); err == nil {
			cmd = exec.Command("c3c", "build", "-C", "--lsp", "--path", root)
			output, _ := cmd.CombinedOutput()
			return parseC3cOutput(string(output), path), nil
		}
	}

	// collect all other .c3 files in the project
	args := []string{"compile-only", "-C", "--lsp"}
	if root != "" {
		otherFiles := collectC3Files(root, path)
		args = append(args, otherFiles...)
	}
	args = append(args, tmpFile.Name())

	cmd = exec.Command("c3c", args...)
	output, _ := cmd.CombinedOutput()
	return parseC3cOutput(string(output), tmpFile.Name()), nil
}

func collectC3Files(root string, excludePath string) []string {
	var files []string
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".c3" && path != excludePath {
			files = append(files, path)
		}
		return nil
	})
	return files
}

func parseC3cOutput(output string, targetPath string) []lsp.Diagnostic {
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

		severity := 1
		if parts[1] == "warning" {
			severity = 2
		}

		filePath := strings.Trim(parts[2], "\"")
		// only show diagnostics for the file that triggered the check
		if !strings.HasSuffix(filePath, strings.TrimPrefix(targetPath, "/")) &&
			filepath.Base(filePath) != filepath.Base(targetPath) {
			continue
		}

		lineNum, err1 := strconv.Atoi(parts[3])
		colNum, err2 := strconv.Atoi(parts[4])
		if err1 != nil || err2 != nil {
			continue
		}

		message := strings.Trim(parts[5], "\"")

		diagnostics = append(diagnostics, lsp.Diagnostic{
			Range: lsp.Range{
				Start: lsp.Position{Line: lineNum - 1, Character: colNum - 1},
				End:   lsp.Position{Line: lineNum - 1, Character: colNum + 2},
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
	// infer type of varName from document
	typeName := inferTypeFromTree(doc, varName)
	if typeName == "" {
		typeName = inferType(doc.Text, varName)
	}
	if typeName == "" {
		return nil
	}

	// look up struct fields
	var items []lsp.CompletionItem
	if info, ok := s.StructIndex[typeName]; ok {
		for _, field := range info.Fields {
			items = append(items, lsp.CompletionItem{
				Label:  field.Name,
				Kind:   lsp.VariableCompletion,
				Detail: field.Type,
			})
		}
	}

	// also look up methods from GlobalIndex
	for _, symbols := range s.GlobalIndex {
		for _, sym := range symbols {
			if strings.HasPrefix(sym.Detail, typeName+".") || strings.HasPrefix(sym.Name, typeName+".") {
				items = append(items, lsp.CompletionItem{
					Label: sym.Name,
					Kind:  lsp.MethodCompletion,
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
