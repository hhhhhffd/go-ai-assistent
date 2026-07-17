package telegram

import (
	"strings"

	"github.com/hhhhhffd/go-gpt-tg-bot/internal/utils"
)

const telegramMessageLimit = 4096

var markdownV2SpecialChars = map[rune]struct{}{
	'_':  {},
	'*':  {},
	'[':  {},
	']':  {},
	'(':  {},
	')':  {},
	'~':  {},
	'`':  {},
	'>':  {},
	'#':  {},
	'+':  {},
	'-':  {},
	'=':  {},
	'|':  {},
	'{':  {},
	'}':  {},
	'.':  {},
	'!':  {},
	'\\': {},
}

func splitTelegramMarkdownV2(text string) []string {
	escaped := escapeMarkdownV2(strings.TrimSpace(text))
	return utils.SplitByLimit(escaped, telegramMessageLimit)
}

func splitTelegramPlain(text string) []string {
	return utils.SplitByLimit(strings.TrimSpace(text), telegramMessageLimit)
}

func escapeMarkdownV2(text string) string {
	var b strings.Builder
	b.Grow(len(text) + len(text)/4)

	for _, ch := range text {
		if _, ok := markdownV2SpecialChars[ch]; ok {
			b.WriteRune('\\')
		}
		b.WriteRune(ch)
	}

	return b.String()
}
