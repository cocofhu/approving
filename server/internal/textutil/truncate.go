package textutil

import "unicode/utf8"

// TruncateBytes returns s unchanged when len(s) <= maxBytes. Otherwise it
// returns the largest valid UTF-8 prefix within the byte budget plus suffix.
// maxBytes applies only to the prefix; suffix is not counted toward the budget.
func TruncateBytes(s string, maxBytes int, suffix string) string {
	if maxBytes < 0 {
		maxBytes = 0
	}
	if len(s) <= maxBytes {
		return s
	}
	prefix := s[:maxBytes]
	for !utf8.ValidString(prefix) && len(prefix) > 0 {
		_, size := utf8.DecodeLastRuneInString(prefix)
		if size == 0 {
			prefix = prefix[:len(prefix)-1]
			continue
		}
		prefix = prefix[:len(prefix)-size]
	}
	return prefix + suffix
}

// TruncateTailBytes returns s unchanged when len(s) <= maxBytes. Otherwise it
// keeps up to maxBytes from the tail of s, discarding leading bytes until the
// slice is valid UTF-8, then prepends prefix.
func TruncateTailBytes(s string, maxBytes int, prefix string) string {
	if maxBytes < 0 {
		maxBytes = 0
	}
	if len(s) <= maxBytes {
		return s
	}
	tail := s[len(s)-maxBytes:]
	for !utf8.ValidString(tail) && len(tail) > 0 {
		tail = tail[1:]
	}
	return prefix + tail
}
