package obs

import (
	"log/slog"
	"regexp"
	"strings"

	"github.com/AthillaZaidan/datanest/backend/internal/migration/sqlguard"
)

const Redacted = "[REDACTED]"

// Matched exactly so harmless keys such as lock_token stay readable.
var sensitiveKeys = map[string]bool{
	"password": true, "passwd": true, "password_hash": true, "sid": true, "session": true, "session_id": true,
	"csrf": true, "csrf_token": true, "token": true, "authorization": true, "cookie": true, "set-cookie": true,
	"set_cookie": true, "secret": true, "credentials_key": true,
}

func Sensitive(key string) bool { return sensitiveKeys[strings.ToLower(key)] }

func ReplaceAttr(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 && a.Key == slog.MessageKey {
		a.Key = AttrEvent
		return a
	}
	return MaskAttr(a)
}

func MaskAttr(a slog.Attr) slog.Attr {
	if Sensitive(a.Key) {
		return slog.String(a.Key, Redacted)
	}
	if a.Key == AttrQuery && a.Value.Kind() == slog.KindString {
		return slog.String(a.Key, MaskSQL(a.Value.String()))
	}
	if a.Key == AttrError {
		return slog.String(a.Key, MaskSecrets(a.Value.String()))
	}
	return a
}

var (
	passwordPair = regexp.MustCompile(`(?i)\b(password|passwd|pwd)(\s*[=:]\s*)\S+`)
	urlUserInfo  = regexp.MustCompile(`://[^/\s:@]+:[^@\s]+@`)
)

// MaskSecrets hides passwords written as password=... and the user:password part of URLs,
// which driver errors can quote.
func MaskSecrets(s string) string {
	s = passwordPair.ReplaceAllString(s, "${1}${2}"+Redacted)
	return urlUserInfo.ReplaceAllString(s, "://"+Redacted+"@")
}

// MaskSQL leaves numeric literals alone: telling them from identifiers needs a real parser.
func MaskSQL(sql string) string { return sqlguard.MaskLiterals(sql) }
