package lsp

type CompletionRequest struct {
	Request
	Params CompletionParams `json:"params"`
}

type CompletionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type CompletionResponse struct {
	Response
	Result []CompletionItem `json:"result"`
}

// CompletionItemKind defines the type icon shown in the Neovim popup

const (
	TextCompletion     = 1
	MethodCompletion   = 2
	FunctionCompletion = 3
	VariableCompletion = 6
	ModuleCompletion   = 9
	EnumCompletion     = 13
	KeywordCompletion  = 14
	ConstantCompletion = 21
	StructCompletion   = 22
)

type CompletionItem struct {
	Label         string `json:"label"`
	Kind          int    `json:"kind,omitempty"`
	Detail        string `json:"detail,omitempty"`
	Documentation string `json:"documentation,omitempty"`
}
