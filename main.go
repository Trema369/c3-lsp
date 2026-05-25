package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"test/lsp/lsp"
	"test/lsp/rpc"
)

func main() {
	logger := getLogger("/home/trema/Projects/learning/golang/lsp/log.txt")
	logger.Println("hey i started")
	fmt.Println("hi")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Split(rpc.Split)
	for scanner.Scan() {
		msg := scanner.Bytes()
		method, contents, err := rpc.DecodeMessage(msg)
		if err != nil {
			logger.Println("got an error", err)
			continue
		}
		handleMessage(logger, method, contents)
	}
}

func handleMessage(logger *log.Logger, method string, contents []byte) {
	logger.Printf("we recieved msg with: %s", method)
	switch method {
	case "initialize":
		var request lsp.InitializeRequest
		if err := json.Unmarshal(contents, &request); err != nil {
			logger.Printf("Hey we could not parse this: %s", err)
		}
		logger.Printf("Connected to : %s %s",
			request.Params.ClientInfo.Name,
			request.Params.ClientInfo.Version)
	}
}

func getLogger(filename string) *log.Logger {
	logfile, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		panic("hey you didnt give me a good file")
	}
	return log.New(logfile, "[test/lsp]", log.Ldate|log.Ltime|log.Lshortfile)
}
