package discord

import (
	"regexp"
	"strings"

	"github.com/hhhhhffd/go-gpt-tg-bot/internal/utils"
)

var roleMentionPattern = regexp.MustCompile(`<@&\d+>`)

func splitDiscordText(text string) []string {
	safe := sanitizeDiscordText(strings.TrimSpace(text))
	return utils.SplitByLimit(safe, DiscordMessageLimit)
}

func sanitizeDiscordText(text string) string {
	replacer := strings.NewReplacer(
		"@everyone", "@\u200beveryone",
		"@here", "@\u200bhere",
	)
	safe := replacer.Replace(text)
	safe = roleMentionPattern.ReplaceAllStringFunc(safe, func(match string) string {
		return strings.Replace(match, "<@&", "<@&\u200b", 1)
	})
	return safe
}

func safeAllowedMentions() *AllowedMentions {
	return &AllowedMentions{
		Parse: []string{},
	}
}
