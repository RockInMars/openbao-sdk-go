package engine

import (
	"errors"
	"strings"
)

var errPath = errors.New("invalid resource path")

func ValidateSegment(s string) error {
	if s == "" || s == "." || s == ".." || s[len(s)-1] == '.' {
		return errPath
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == '.') {
			return errPath
		}
	}
	return nil
}
func ValidatePath(s string) error {
	if s == "" {
		return errPath
	}
	for _, seg := range strings.Split(s, "/") {
		if ValidateSegment(seg) != nil {
			return errPath
		}
	}
	return nil
}
