// Package sqlguard rejects transaction control statements from user migrations.
package sqlguard

import (
	"strings"
	"unicode"

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
			for i < len(sql) && isWord(sql[i]) {
				i++
			}
			out = append(out, strings.ToLower(sql[start:i]))
		default:
			i++
		}
	}
	return out
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
		for i < len(sql) && sql[i] != '\n' {
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
	case sql[i] == '$':
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

func isWord(b byte) bool { return b == '_' || unicode.IsLetter(rune(b)) || unicode.IsDigit(rune(b)) }
func dollarEnd(sql string, i int) string {
	j := i + 1
	for j < len(sql) && (sql[j] == '_' || unicode.IsLetter(rune(sql[j])) || unicode.IsDigit(rune(sql[j]))) {
		j++
	}
	if j < len(sql) && sql[j] == '$' {
		return sql[i : j+1]
	}
	return ""
}
