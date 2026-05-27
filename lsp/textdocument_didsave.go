package lsp

// in lsp/textdocument_didchange.go or a new file
type DidSaveTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type DidSaveTextDocumentNotification struct {
	Notification
	Params DidSaveTextDocumentParams `json:"params"`
}
