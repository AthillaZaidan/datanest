package sqlguard

const (
	batchOneStatement = "SQL per-batch harus tepat satu statement."
	batchBadParameter = "SQL per-batch hanya boleh memakai parameter $1 dan $2."
	batchMissing      = "SQL per-batch harus memakai $1 (batas bawah, tidak termasuk) dan $2 (batas atas, termasuk)."
)

func BatchProblem(sql string) string {
	if len(Split(sql)) != 1 {
		return batchOneStatement
	}
	var lower, upper bool
	for i := 0; i < len(sql); {
		if j := skip(sql, i); j != i {
			i = j
			continue
		}
		if sql[i] != '$' || i > 0 && (isWord(sql[i-1]) || sql[i-1] == '$') {
			i++
			continue
		}
		j := i + 1
		for j < len(sql) && sql[j] >= '0' && sql[j] <= '9' {
			j++
		}
		if j == i+1 {
			i++
			continue
		}
		switch sql[i+1 : j] {
		case "1":
			lower = true
		case "2":
			upper = true
		default:
			return batchBadParameter
		}
		i = j
	}
	if !lower || !upper {
		return batchMissing
	}
	return ""
}
