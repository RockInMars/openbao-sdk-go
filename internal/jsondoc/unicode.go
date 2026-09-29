package jsondoc

// validUnicodeEscapes rejects UTF-16 code units that encoding/json would
// silently replace with U+FFFD. It does not normalize valid Unicode or change
// the standard JSON grammar; the decoder still validates that grammar.
func validUnicodeEscapes(raw []byte) bool {
	quoted := false
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case '"':
			quoted = !quoted
		case '\\':
			if !quoted {
				return false
			}
			i++
			if i >= len(raw) {
				return false
			}
			if raw[i] != 'u' {
				continue
			}
			unit, ok := hexUnit(raw, i+1)
			if !ok {
				return false
			}
			i += 4
			switch {
			case unit >= 0xd800 && unit <= 0xdbff:
				if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
					return false
				}
				low, ok := hexUnit(raw, i+3)
				if !ok || low < 0xdc00 || low > 0xdfff {
					return false
				}
				i += 6
			case unit >= 0xdc00 && unit <= 0xdfff:
				return false
			}
		}
	}
	return !quoted
}

func hexUnit(raw []byte, start int) (uint16, bool) {
	if start > len(raw)-4 {
		return 0, false
	}
	var value uint16
	for _, c := range raw[start : start+4] {
		value <<= 4
		switch {
		case c >= '0' && c <= '9':
			value += uint16(c - '0')
		case c >= 'a' && c <= 'f':
			value += uint16(c-'a') + 10
		case c >= 'A' && c <= 'F':
			value += uint16(c-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}
