package analysis

import (
	"os"
	"path/filepath"
)

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
