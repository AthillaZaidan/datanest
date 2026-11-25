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
	for _, t := range lex(sql) {
		switch t.kind {
		case kindSemicolon:
			out = append(out, ";")
		case kindWord, kindNumber:
			out = append(out, strings.ToLower(sql[t.start:t.end]))
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
	for _, t := range lex(sql) {
		if t.kind == kindSemicolon {
			add(t.start)
			start = t.end
		}
	}
	add(len(sql))
	return out
}

// MaskLiterals replaces string literals with '?' and drops comments.
func MaskLiterals(sql string) string {
	var b strings.Builder
	for _, t := range lex(sql) {
		switch t.kind {
		case kindString, kindDollar:
			b.WriteString("'?'")
		case kindComment:
			b.WriteByte(' ')
		default:
			b.WriteString(sql[t.start:t.end])
		}
	}
	return b.String()
}

type kind int

const (
	kindOther kind = iota
	kindSpace
	kindSemicolon
	kindWord
	kindNumber
	kindParam
	kindString
	kindQuotedIdent
	kindDollar
	kindComment
)

type token struct {
	kind       kind
	start, end int
}

func lex(sql string) []token {
	var out []token
	for i := 0; i < len(sql); {
		kind, end := next(sql, i)
		out = append(out, token{kind, i, end})
		if kind == kindWord && isEscapePrefix(sql[i:end]) && end < len(sql) && sql[end] == '\'' {
			stringEnd := escapeStringEnd(sql, end+1)
			out = append(out, token{kindString, end, stringEnd})
			end = stringEnd
		}
		i = end
	}
	return out
}

func next(sql string, i int) (kind, int) {
	c := sql[i]
	switch {
	case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v':
		return kindSpace, i + 1
	case c == ';':
		return kindSemicolon, i + 1
	case strings.HasPrefix(sql[i:], "--"):
		return kindComment, lineCommentEnd(sql, i)
	case strings.HasPrefix(sql[i:], "/*"):
		return kindComment, blockCommentEnd(sql, i)
	case isTagStart(c):
		j := i + 1
		for j < len(sql) && (isWord(sql[j]) || sql[j] == '$') {
			j++
		}
		return kindWord, j
	case isDigit(c) || c == '.' && i+1 < len(sql) && isDigit(sql[i+1]):
		return kindNumber, numberEnd(sql, i)
	case c == '\'':
		return kindString, quotedEnd(sql, i)
	case c == '"':
		return kindQuotedIdent, quotedEnd(sql, i)
	case c == '$':
		if j := digitsEnd(sql, i+1); j > i+1 {
			return kindParam, j
		}
		if end := dollarEnd(sql, i); end != "" {
			body := i + len(end)
			if close := strings.Index(sql[body:], end); close >= 0 {
				return kindDollar, body + close + len(end)
			}
			return kindDollar, len(sql)
		}
	}
	return kindOther, i + 1
}

func lineCommentEnd(sql string, i int) int {
	for i < len(sql) && sql[i] != '\n' && sql[i] != '\r' {
		i++
	}
	return i
}

func blockCommentEnd(sql string, i int) int {
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
	return i
}

func quotedEnd(sql string, i int) int {
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
	return i
}

func digitsEnd(sql string, i int) int {
	for i < len(sql) && isDigit(sql[i]) {
		i++
	}
	return i
}

func numberEnd(sql string, i int) int {
	i = digitsEnd(sql, i)
	if i < len(sql) && sql[i] == '.' {
		i = digitsEnd(sql, i+1)
	}
	if i < len(sql) && (sql[i] == 'e' || sql[i] == 'E') {
		j := i + 1
		if j < len(sql) && (sql[j] == '+' || sql[j] == '-') {
			j++
		}
		if k := digitsEnd(sql, j); k > j {
			i = k
		}
	}
	return i
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func isWord(b byte) bool { return isTagStart(b) || isDigit(b) }

func isTagStart(b byte) bool {
	return b == '_' || b >= 0x80 || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

func isEscapePrefix(word string) bool { return word == "e" || word == "E" }

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
			i = lineCommentEnd(sql, i)
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
