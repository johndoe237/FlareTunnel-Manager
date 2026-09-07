// Package logging provides the manager's operational logging helpers.
//
// Logs are human-readable and operations-oriented. Credentials, API tokens
// and secrets are never logged.
package logging

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Logger writes operational log lines to stdout (info) and stderr (error).
type Logger struct{}

// New returns a Logger.
func New() *Logger { return &Logger{} }

// Infof writes an informational log line.
func (l *Logger) Infof(format string, args ...any) {
	fmt.Printf("[info] %s\n", sanitize(fmt.Sprintf(format, args...)))
}

// Errorf writes an error log line.
func (l *Logger) Errorf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[error] %s\n", sanitize(fmt.Sprintf(format, args...)))
}

func sanitize(s string) string {
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || value == "" || !sensitiveKey(key) {
			continue
		}
		s = strings.ReplaceAll(s, value, "[REDACTED]")
	}
	field := regexp.MustCompile(`(?i)(api[_-]?token|token|secret|password|credential)(["' ]*[:=]["' ]*)[^,\s}"']+`)
	return field.ReplaceAllString(s, `$1$2[REDACTED]`)
}

func sensitiveKey(key string) bool {
	key = strings.ToLower(key)
	return strings.Contains(key, "token") || strings.Contains(key, "secret") || strings.Contains(key, "password") || strings.Contains(key, "credential") || strings.Contains(key, "api_key") || strings.Contains(key, "apikey")
}
