package domain

import (
	"crypto/rand"
	"encoding/base32"
	"strings"
	"unicode"
)

func NewID(prefix string) string {
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		panic(err)
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(random)
	return strings.Trim(prefix, "-_") + "_" + strings.ToLower(encoded)
}

func Slugify(value string) string {
	var result strings.Builder
	dash := false
	for _, character := range strings.ToLower(strings.TrimSpace(value)) {
		switch {
		case unicode.IsLetter(character) || unicode.IsDigit(character):
			result.WriteRune(character)
			dash = false
		case result.Len() > 0 && !dash:
			result.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(result.String(), "-")
}
