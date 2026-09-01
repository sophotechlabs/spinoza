package testguard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestEveryGoroutineOwningPackageChecksLeaks(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the repository")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(here), "..", ".."))
	owners := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path == root {
				return nil
			}
			name := entry.Name()
			if strings.HasPrefix(name, ".") || name == "frontend" || name == "test" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		if ownsGoroutine(parsed) {
			owners[filepath.Dir(path)] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan Go packages: %v", err)
	}

	directories := make([]string, 0, len(owners))
	for directory := range owners {
		directories = append(directories, directory)
	}
	slices.Sort(directories)
	for _, directory := range directories {
		body, readErr := os.ReadFile(filepath.Join(directory, "leak_test.go"))
		if readErr != nil {
			t.Errorf("%s owns goroutines but has no leak_test.go: %v", relative(root, directory), readErr)
			continue
		}
		if !bytesContainAll(body, "func TestMain", "goleak.VerifyTestMain") {
			t.Errorf("%s owns goroutines but leak_test.go does not verify TestMain", relative(root, directory))
		}
	}
}

func ownsGoroutine(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		if found {
			return false
		}
		if _, ok := node.(*ast.GoStmt); ok {
			found = true
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if ok && selector.Sel.Name == "Go" {
			found = true
			return false
		}
		return true
	})
	return found
}

func bytesContainAll(body []byte, parts ...string) bool {
	text := string(body)
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}

func relative(root, path string) string {
	found, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	if found == "." {
		return "root"
	}
	return found
}
