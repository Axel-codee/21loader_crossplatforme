package util

func ParseArgumentString(input string) []string {
	args := []string{}
	current := make([]rune, 0, len(input))
	var quote rune
	escaped := false

	flush := func() {
		if len(current) == 0 {
			return
		}
		args = append(args, string(current))
		current = current[:0]
	}

	for _, ch := range input {
		if escaped {
			current = append(current, ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if ch == quote {
				quote = 0
			} else {
				current = append(current, ch)
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			flush()
			continue
		}
		current = append(current, ch)
	}
	flush()
	return args
}
