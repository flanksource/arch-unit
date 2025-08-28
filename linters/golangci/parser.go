package golangci

import (
	"strconv"
	"strings"
)

// cleanGolangciMessage cleans up golangci-lint messages by removing various prefixes
func cleanGolangciMessage(message string) string {
	// Strip various prefixes from golangci-lint messages
	// 1. Strip package info prefixes like ": # github.com/flanksource/arch-unit/examples/go-project/pkg/service"
	if strings.HasPrefix(message, ": # ") {
		if newlineIdx := strings.Index(message, "\n"); newlineIdx != -1 {
			message = message[newlineIdx+1:]
		}
	}

	// 2. Strip complex import error prefixes like "could not import ... (-: # ...\n...)"
	if strings.Contains(message, "could not import") && strings.Contains(message, "(-: #") {
		if parenIdx := strings.Index(message, "(-:"); parenIdx != -1 {
			beforeParen := strings.TrimSpace(message[:parenIdx])
			if parenCloseIdx := strings.Index(message[parenIdx:], ")"); parenCloseIdx != -1 {
				afterParen := strings.TrimSpace(message[parenIdx+parenCloseIdx+1:])
				if afterParen != "" {
					message = beforeParen + " (" + afterParen + ")"
				} else {
					message = beforeParen
				}
			}
		}
	}

	// 3. Strip location prefix from message text
	// Look for patterns like "./file.go:line:col: " or "file.go:line:col: "
	if idx := strings.LastIndex(message, "\n"); idx != -1 {
		// For multi-line messages, look for location in the last line
		lastLine := message[idx+1:]
		if locIdx := strings.Index(lastLine, ": "); locIdx != -1 {
			// Check if this looks like a file location (has colons before the ": ")
			prefix := lastLine[:locIdx]
			if strings.Count(prefix, ":") >= 2 || strings.HasPrefix(prefix, "./") {
				// Strip the location prefix
				message = message[:idx+1] + lastLine[locIdx+2:]
			}
		}
	} else {
		// Single line message
		// Strip patterns like "./file.go:6:2: " or "file.go:6:2: "
		if strings.HasPrefix(message, "./") || strings.HasPrefix(message, "../") {
			if idx := strings.Index(message, ": "); idx != -1 {
				prefix := message[:idx]
				if strings.Count(prefix, ":") >= 2 {
					message = message[idx+2:]
				}
			}
		} else if idx := strings.Index(message, ": "); idx != -1 {
			// Check if prefix looks like "file.go:line:col"
			prefix := message[:idx]
			if strings.Count(prefix, ":") == 2 && !strings.Contains(prefix, " ") {
				// Likely a file location, strip it
				message = message[idx+2:]
			}
		}
	}

	return strings.TrimSpace(message)
}

// parseInt safely parses a string to int, returning 0 on error
func parseInt(s string) int {
	if i, err := strconv.Atoi(s); err == nil {
		return i
	}
	return 0
}
