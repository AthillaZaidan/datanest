// Package sqlguard rejects transaction control statements from user migrations.
package sqlguard

import (
	"strings"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
)

// Check tokenizes enough of PostgreSQL SQL to ignore words in quoted strings,
// identifiers, comments, and dollar-quoted function bodies. Only the leading word of
// each statement matters, so keywords like CASE ... END stay allowed.
func Check(sql string) error {
	words := tokens(sql)
	for i, word := range words {
		if i > 0 && words[i-1] != ";" {
			continue
		}
		next := ""
		if i+1 < len(words) {
			next = words[i+1]
		}
		switch word {
		case "begin", "commit", "end", "rollback", "abort", "savepoint", "release":
			return apperr.ErrTransactionControlNotAllowed
		case "start", "prepare":
			if next == "transaction" {
				return apperr.ErrTransactionControlNotAllowed
			}
		}
	}
	return nil
}

func tokens(sql string) []string {
	var out []string
	for i := 0; i < len(sql); {
		if j := skip(sql, i); j != i {
			i = j
			continue
		}
		switch {
		case sql[i] == ';':
			out = append(out, ";")
			i++
		case isWord(sql[i]):
			start := i
			for i < len(sql) && (isWord(sql[i]) || sql[i] == '$') {
				i++
			}
			out = append(out, strings.ToLower(sql[start:i]))
		default:
			i++
		}
	}
	return out
}

// FirstWord is the lower-case leading keyword of a statement, ignoring comments.
func FirstWord(stmt string) string {
	if w := tokens(stmt); len(w) > 0 {
		return w[0]
	}
	return ""
}

// Split cuts sql into statements at top-level semicolons, keeping each statement's text as written.
func Split(sql string) []string {
	var out []string
	start := 0
	add := func(end int) {
		if piece := strings.TrimSpace(sql[start:end]); piece != "" {
			out = append(out, piece)
		}
	}
	for i := 0; i < len(sql); {
		if j := skip(sql, i); j != i {
			i = j
			continue
		}
		if sql[i] == ';' {
			add(i)
			start = i + 1
		}
		i++
	}
	add(len(sql))
	return out
}

// skip returns the index after a comment, quoted string, quoted identifier or dollar-quoted body starting at i,
// or i itself when none starts there.
func skip(sql string, i int) int {
	switch {
	case strings.HasPrefix(sql[i:], "--"):
		i += 2
		for i < len(sql) && sql[i] != '\n' && sql[i] != '\r' {
			i++
		}
	case strings.HasPrefix(sql[i:], "/*"):
		i += 2
		depth := 1
		for i < len(sql) && depth > 0 {
			if strings.HasPrefix(sql[i:], "/*") {
				depth++
				i += 2
			} else if strings.HasPrefix(sql[i:], "*/") {
				depth--
				i += 2
			} else {
				i++
			}
		}
	case sql[i] == '\'' && endsEscapePrefix(sql, i):
		i = escapeStringEnd(sql, i+1)
	case sql[i] == '\'' || sql[i] == '"':
		q := sql[i]
		i++
		for i < len(sql) {
			if sql[i] == q {
				i++
				if i < len(sql) && sql[i] == q {
					i++
					continue
				}
				break
			}
			i++
		}
	case sql[i] == '$' && (i == 0 || !isWord(sql[i-1]) && sql[i-1] != '$'):
		if end := dollarEnd(sql, i); end != "" {
			i += len(end)
			if close := strings.Index(sql[i:], end); close >= 0 {
				i += close + len(end)
			} else {
				i = len(sql)
			}
		}
	}
	return i
}

// MaskLiterals replaces string literals with '?' and drops comments.
func MaskLiterals(sql string) string {
	var b strings.Builder
	for i := 0; i < len(sql); {
		j := skip(sql, i)
		if j == i {
			b.WriteByte(sql[i])
			i++
			continue
		}
		switch seg := sql[i:j]; {
		case seg[0] == '\'' || seg[0] == '$':
			b.WriteString("'?'")
		case seg[0] == '-' || seg[0] == '/':
			b.WriteByte(' ')
		default:
			b.WriteString(seg)
		}
		i = j
	}
	return b.String()
}

func isWord(b byte) bool {
	return b == '_' || b >= 0x80 || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}

func isTagStart(b byte) bool {
	return b == '_' || b >= 0x80 || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

func endsEscapePrefix(sql string, quote int) bool {
	start := quote
	for start > 0 && (isWord(sql[start-1]) || sql[start-1] == '$') {
		start--
	}
	return quote-start == 1 && (sql[start] == 'e' || sql[start] == 'E')
}

func escapeStringEnd(sql string, i int) int {
	for i < len(sql) {
		switch sql[i] {
		case '\\':
			i += 2
		case '\'':
			i++
			if i < len(sql) && sql[i] == '\'' {
				i++
				continue
			}
			next, ok := continuation(sql, i)
			if !ok {
				return i
			}
			i = next
		default:
			i++
		}
	}
	return len(sql)
}

func continuation(sql string, i int) (int, bool) {
	sawNewline := false
	for i < len(sql) {
		switch {
		case sql[i] == '\n' || sql[i] == '\r':
			sawNewline = true
			i++
		case sql[i] == ' ' || sql[i] == '\t' || sql[i] == '\f' || sql[i] == '\v':
			i++
		case strings.HasPrefix(sql[i:], "--"):
			for i < len(sql) && sql[i] != '\n' && sql[i] != '\r' {
				i++
			}
		case sql[i] == '\'':
			return i + 1, sawNewline
		default:
			return i, false
		}
	}
	return i, false
}

func dollarEnd(sql string, i int) string {
	j := i + 1
	if j < len(sql) && isTagStart(sql[j]) {
		for j < len(sql) && isWord(sql[j]) {
			j++
		}
	}
	if j < len(sql) && sql[j] == '$' {
		return sql[i : j+1]
	}
	return ""
}
