package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// allowCommentPrefix starts a comment line in an --allow-removals file.
const allowCommentPrefix = "#"

// byteOrderMark is what some editors put before a file's first line.
const byteOrderMark = "\ufeff"

// readAllowedRemovals reads the public paths an import may remove, one per
// line; blank and comment lines are skipped. No file allows nothing.
func readAllowedRemovals(file string) (map[string]bool, error) {
	allowed := map[string]bool{}
	if file == "" {
		return allowed, nil
	}
	f, err := os.Open(filepath.Clean(file))
	if err != nil {
		return nil, fmt.Errorf("read --allow-removals: %w", err)
	}
	defer func() { _ = f.Close() }()
	lines := bufio.NewScanner(f)
	for lines.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(lines.Text(), byteOrderMark))
		if line != "" && !strings.HasPrefix(line, allowCommentPrefix) {
			allowed[line] = true
		}
	}
	if err := lines.Err(); err != nil {
		return nil, fmt.Errorf("read --allow-removals %s: %w", file, err)
	}
	return allowed, nil
}
