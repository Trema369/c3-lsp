package main

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"test/lsp/analysis"
	"test/lsp/lsp"
	"test/lsp/rpc"
)

var writeMu sync.Mutex
var (
	diagnosticTimers = make(map[string]*time.Timer)
	timersMu         sync.Mutex
)

func main() {
	logger := getLogger("/home/trema/Projects/learning/golang/lsp/log.txt")

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	scanner.Split(rpc.Split)

	state := analysis.NewState()
	writer := os.Stdout

	for scanner.Scan() {
		msg := scanner.Bytes()
		method, contents, err := rpc.DecodeMessage(msg)
		if err != nil {
			logger.Println("got an error", err)
			continue
		}
		handleMessage(logger, writer, state, method, contents)
	}
}

func handleMessage(logger *log.Logger, writer io.Writer, state *analysis.State, method string, contents []byte) {
	logger.Printf("we received msg with: %s", method)
	switch method {
	case "initialize":
		var request lsp.InitializeRequest
		if err := json.Unmarshal(contents, &request); err != nil {
			logger.Printf("Hey we could not parse this: %s", err)
		}
		logger.Printf("Connected to : %s %s",
			request.Params.ClientInfo.Name,
			request.Params.ClientInfo.Version)

		msg := lsp.NewInitalizeResponse(request.ID)
		writeResponse(writer, msg)
		logger.Print("send the reply")

		go func() {
			stdLibPath := "/usr/local/lib/c3/"

			logger.Printf("Indexing C3 standard library path: %s", stdLibPath)
			if err := state.IndexDirectory(stdLibPath); err != nil {
				logger.Printf("Global library index failed: %s", err)
			} else {
				modCount := state.GetGlobalIndexCount()
				logger.Printf("Global index built successfully! Cached %d active submodules.", modCount)
			}
		}()

	case "textDocument/didOpen":
		var request lsp.DidOpenTextDocumentNotification
		if err := json.Unmarshal(contents, &request); err != nil {
			logger.Printf("textDocument/didOpen: %s", err)
		}
		uri := request.Params.TextDocument.URI
		logger.Printf("Opened: %s", uri)
		state.OpenDocument(uri, request.Params.TextDocument.Text)

		// index project if we have a root
		go func() {
			path := strings.TrimPrefix(uri, "file://")
			root := FindProjectRoot(path)
			if root != "" {
				logger.Printf("Indexing project at: %s", root)
				if err := state.IndexProject(root); err != nil {
					logger.Printf("IndexProject failed: %s", err)
				} else {
					logger.Printf("Project indexed successfully")
				}
			}
		}()

		go triggerDiagnostics(writer, logger, state, uri, true)
	case "textDocument/didChange":
		var request lsp.TextDocumentDidChangeNotification
		if err := json.Unmarshal(contents, &request); err != nil {
			logger.Printf("textDocument/didChange: %s", err)
		}
		logger.Printf("Changed: %s", request.Params.TextDocument.URI)
		for _, change := range request.Params.ContentChanges {
			state.UpdateDocument(request.Params.TextDocument.URI, change.Text)
		}

		scheduleDiagnostics(writer, logger, state, request.Params.TextDocument.URI, false)
	case "textDocument/didSave":
		var request lsp.DidSaveTextDocumentNotification
		if err := json.Unmarshal(contents, &request); err != nil {
			logger.Printf("textDocument/didSave: %s", err)
		}
		uri := request.Params.TextDocument.URI
		go func() {
			path := strings.TrimPrefix(uri, "file://")
			root := analysis.FindProjectRoot(path)
			if root != "" {
				state.IndexProject(root)
			}
		}()
		go triggerDiagnostics(writer, logger, state, uri, true)

	case "textDocument/hover":
		var request lsp.HoverRequest
		if err := json.Unmarshal(contents, &request); err != nil {
			logger.Printf("textDocument/hover: %s", err)
			return
		}
		response := lsp.HoverResponse{
			Response: lsp.Response{
				RPC: "2.0",
				ID:  request.ID,
			},
			Result: lsp.HoverResult{
				Contents: "Hello from lsp",
			},
		}
		writeResponse(writer, response)
	case "textDocument/completion":
		var request lsp.CompletionRequest
		if err := json.Unmarshal(contents, &request); err != nil {
			logger.Printf("textDocument/completion parsing failed: %s", err)
			return
		}

		items := state.GetCompletionItems(request.Params.TextDocument.URI, request.Params.Position)

		response := lsp.CompletionResponse{
			Response: lsp.Response{
				RPC: "2.0",
				ID:  request.ID,
			},
			Result: items,
		}

		writeResponse(writer, response)
		logger.Printf("Dispatched %d completion items back to client", len(items))
	}
}

func scheduleDiagnostics(writer io.Writer, logger *log.Logger, state *analysis.State, uri string, fullBuild bool) {
	timersMu.Lock()
	defer timersMu.Unlock()
	if timer, exists := diagnosticTimers[uri]; exists {
		timer.Stop()
	}
	diagnosticTimers[uri] = time.AfterFunc(300*time.Millisecond, func() {
		triggerDiagnostics(writer, logger, state, uri, fullBuild)
	})
}

func triggerDiagnostics(writer io.Writer, logger *log.Logger, state *analysis.State, uri string, fullBuild bool) {
	diagnostics, err := state.Analyze(uri, fullBuild)
	if err != nil {
		logger.Printf("Analysis failed for %s: %s", uri, err)
		return
	}

	notification := lsp.PublishDiagnosticsNotification{
		Notification: lsp.Notification{
			RPC:    "2.0",
			Method: "textDocument/publishDiagnostics",
		},
		Params: lsp.PublishDiagnosticsParams{
			URI:         uri,
			Diagnostics: diagnostics,
		},
	}
	debugBytes, _ := json.Marshal(notification)
	logger.Printf("ULTIMATE JSON TRUTH: %s", string(debugBytes))

	writeResponse(writer, notification)
	logger.Printf("Dispatched diagnostics notification back to client for: %s", uri)
}

func writeResponse(writer io.Writer, msg any) {
	writeMu.Lock()
	defer writeMu.Unlock()

	reply := rpc.EncodeMessage(msg)
	writer.Write([]byte(reply))
}

func getLogger(filename string) *log.Logger {
	logfile, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o666)
	if err != nil {
		panic("hey you didnt give me a good file")
	}
	return log.New(logfile, "[test/lsp]", log.Ldate|log.Ltime|log.Lshortfile)
}

func FindProjectRoot(filePath string) string {
	dir := filepath.Dir(filePath)
	for {
		if _, err := os.Stat(filepath.Join(dir, "project.json")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// no project.json found, use the file's directory
			return filepath.Dir(filePath)
		}
		dir = parent
	}
}
