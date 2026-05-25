package lsp

type InitializeRequest struct {
	Request
	Params InitializeRequestParams `json:"params"`
}

type InitializeRequestParams struct {
	ClientInfo *ClientInfo `json:"clientInfo"`
}

type ClientInfo struct {
	Name    string `jason:"name"`
	Version string `json:"version"`
}

type InitializeResponse struct {
	Response
	Result InitializeResult `json:"result"`
}

type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
	ServerInfo   ServerInfo         `json:"serverInfo"`
}

type ServerCapabilities struct{}
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func NewInitalizeResponse(id int) InitializeResponse {
	return InitializeResponse{
		Response: Response{
			RPC: "2.0",
			ID:  id,
		},
		Result: InitializeResult{
			Capabilities: ServerCapabilities{},
			ServerInfo: ServerInfo{
				Name:    "test/lsp",
				Version: "0.0",
			},
		},
	}
}
