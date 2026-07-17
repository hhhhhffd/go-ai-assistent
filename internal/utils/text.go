package utils

import (
	"regexp"
	"strings"
)

func ContainsMention(text, username string) bool {
	if strings.TrimSpace(text) == "" || strings.TrimSpace(username) == "" {
		return false
	}

	pattern := regexp.MustCompile(`(?i)@` + regexp.QuoteMeta(strings.TrimSpace(username)) + `\b`)
	return pattern.FindStringIndex(text) != nil
}

func StripMention(text, username string) string {
	if strings.TrimSpace(text) == "" || strings.TrimSpace(username) == "" {
		return strings.TrimSpace(text)
	}

	pattern := regexp.MustCompile(`(?i)@` + regexp.QuoteMeta(strings.TrimSpace(username)) + `\b`)
	clean := pattern.ReplaceAllString(text, " ")
	clean = strings.Join(strings.Fields(clean), " ")
	return strings.TrimSpace(clean)
}
