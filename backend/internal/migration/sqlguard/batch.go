package sqlguard

const (
	batchOneStatement = "SQL per-batch harus tepat satu statement."
	batchBadParameter = "SQL per-batch hanya boleh memakai parameter $1 dan $2."
	batchMissing      = "SQL per-batch harus memakai $1 (batas bawah, tidak termasuk) dan $2 (batas atas, termasuk)."
)

func BatchProblem(sql string) string {
	statements := 0
	for _, piece := range Split(sql) {
		if hasToken(piece) {
			statements++
		}
	}
	if statements != 1 {
		return batchOneStatement
	}
	var lower, upper bool
	for _, t := range lex(sql) {
		if t.kind != kindParam {
			continue
		}
		switch sql[t.start+1 : t.end] {
		case "1":
			lower = true
		case "2":
			upper = true
		default:
			return batchBadParameter
		}
	}
	if !lower || !upper {
		return batchMissing
	}
	return ""
}

func hasToken(sql string) bool {
	for _, t := range lex(sql) {
		if t.kind != kindSpace && t.kind != kindComment {
			return true
		}
	}
	return false
}
