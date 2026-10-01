package config

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
)

// syntaxError retains tomllib diagnostics for common malformed configurations.
// Unrecognized parser errors keep their technical diagnostic rather than guessing a repair.
func syntaxError(src string, err error) string {
	p, ok := err.(toml.ParseError)
	if !ok {
		return err.Error()
	}
	pos := min(p.Position.Start, len(src))
	message := p.Message
	lineStart := strings.LastIndex(src[:pos], "\n") + 1
	lineEnd := strings.Index(src[lineStart:], "\n")
	if lineEnd < 0 {
		lineEnd = len(src)
	} else {
		lineEnd += lineStart
	}
	line := src[lineStart:lineEnd]
	switch {
	case strings.HasPrefix(message, "expected value"), message == "unexpected EOF; expected value", strings.HasPrefix(message, "floats must start"):
		message = "Invalid value"
		if strings.Contains(p.Message, "EOF") {
			pos = len(src)
		}
	case strings.HasPrefix(message, "expected '.' or '='"), message == "unexpected EOF; expected key separator '='":
		message = "Expected '=' after a key in a key/value pair"
		if strings.Contains(p.Message, "EOF") {
			pos = len(src)
		}
	case strings.HasPrefix(message, "expected '.' or ']'"), strings.HasPrefix(message, "expected end of table array"):
		message = "Expected ']' at the end of a table declaration"
		if strings.HasPrefix(strings.TrimSpace(line), "[[") {
			message = "Expected ']]' at the end of an array declaration"
		}
	case strings.HasPrefix(message, "expected a comma (',') or array terminator"):
		message = "Unclosed array"
		if strings.Contains(p.Message, "EOF") {
			pos = len(src)
		}
	case strings.HasPrefix(message, "expected a comma or an inline table terminator"), message == "newlines not allowed within inline tables":
		message = "Unclosed inline table"
		if strings.Contains(p.Message, "EOF") {
			pos = len(src)
		}
	case strings.HasPrefix(message, "unexpected EOF; expected"):
		message = "Unterminated string"
		pos = len(src)
	case message == "strings cannot contain newlines":
		message = "Illegal character '\\n'"
	case strings.HasPrefix(message, "invalid escape in string"):
		message = "Unescaped '\\' in a string"
		pos = min(pos+p.Position.Len, len(src))
	case message == "trailing comma not allowed in inline tables", strings.HasPrefix(message, "unexpected end of table name"), strings.HasPrefix(message, "unexpected table separator"), strings.HasPrefix(message, "unexpected '='"), strings.HasPrefix(message, "unexpected '.'"):
		message = "Invalid initial character for a key part"
	case strings.HasPrefix(message, "expected a top-level item"):
		message = "Expected newline or end of document after a statement"
		for pos < len(src) && (src[pos] == ' ' || src[pos] == '\t') {
			pos++
		}
	case strings.HasPrefix(message, "Invalid integer") && strings.Contains(message, "leading zeroes"):
		message = "Expected newline or end of document after a statement"
		pos++
	case strings.HasPrefix(message, "Key '") && strings.Contains(message, "already been defined"):
		if strings.HasPrefix(strings.TrimSpace(line), "[") {
			name := strings.Trim(strings.TrimSpace(line), "[] ")
			parts := strings.Split(name, ".")
			var quoted []string
			for _, part := range parts {
				quoted = append(quoted, "'"+strings.Trim(part, "\"'")+"'")
			}
			tuple := strings.Join(quoted, ", ")
			if len(parts) == 1 {
				tuple += ","
			}
			message = "Cannot declare (" + tuple + ") twice"
			pos = lineStart + strings.LastIndex(line, "]")
		} else {
			message = "Cannot overwrite a value"
			pos = lineEnd
			for pos > lineStart && (src[pos-1] == ' ' || src[pos-1] == '\t') {
				pos--
			}
		}
	default:
		return strings.ReplaceAll(err.Error(), "\n", "; ")
	}
	if pos >= len(src) {
		return message + " (at end of document)"
	}
	lineno := strings.Count(src[:pos], "\n") + 1
	start := strings.LastIndex(src[:pos], "\n") + 1
	col := utf8.RuneCountInString(src[start:pos]) + 1
	return fmt.Sprintf("%s (at line %d, column %d)", message, lineno, col)
}
