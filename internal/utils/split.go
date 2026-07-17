package utils

import "strings"

func SplitByLimit(text string, limit int) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	if limit <= 0 {
		return []string{text}
	}

	runes := []rune(text)
	if len(runes) <= limit {
		return []string{text}
	}

	chunks := make([]string, 0, len(runes)/limit+1)
	for len(runes) > 0 {
		if len(runes) <= limit {
			chunk := strings.TrimSpace(string(runes))
			if chunk != "" {
				chunks = append(chunks, chunk)
			}
			break
		}

		cut := findCutIndex(runes, limit)
		if cut <= 0 {
			cut = limit
		}

		for cut > 1 && runes[cut-1] == '\\' {
			cut--
		}
		if cut == 0 {
			cut = limit
		}

		chunk := strings.TrimSpace(string(runes[:cut]))
		if chunk != "" {
			chunks = append(chunks, chunk)
		}

		runes = trimLeadingWhitespaces(runes[cut:])
	}

	return chunks
}

func findCutIndex(runes []rune, limit int) int {
	if len(runes) <= limit {
		return len(runes)
	}

	searchFrom := limit - 1
	searchTo := limit - 220
	if searchTo < 0 {
		searchTo = 0
	}

	for i := searchFrom; i >= searchTo; i-- {
		if runes[i] == '\n' || runes[i] == ' ' || runes[i] == '\t' {
			return i + 1
		}
		if runes[i] == '.' || runes[i] == '!' || runes[i] == '?' || runes[i] == ';' || runes[i] == ':' {
			return i + 1
		}
	}

	return limit
}

func trimLeadingWhitespaces(input []rune) []rune {
	index := 0
	for index < len(input) {
		if input[index] != ' ' && input[index] != '\n' && input[index] != '\t' && input[index] != '\r' {
			break
		}
		index++
	}
	return input[index:]
}
