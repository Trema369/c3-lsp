package parser

/*
#cgo CFLAGS: -I./c_src
#include "c_src/c3_parser.c"
#include "c_src/c3_scanner.c"

const TSLanguage *tree_sitter_c3(void);
*/
import "C"
import (
	"unsafe"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func GetLanguage() *sitter.Language {
	ptr := unsafe.Pointer(C.tree_sitter_c3())
	return sitter.NewLanguage(ptr)
}
