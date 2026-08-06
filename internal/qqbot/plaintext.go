package qqbot

import (
	"fmt"
	"regexp"
	"strings"
)

const codeBlockMarker = "\x00PST_CODE_BLOCK_%d\x00"

var (
	imageLinkPattern      = regexp.MustCompile(`!\[([^\]]*)\]\(([^)\s]+)\)`)
	markdownLinkPattern   = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)
	inlineCodePattern     = regexp.MustCompile("`([^`\n]+)`")
	cjkSpacePattern       = regexp.MustCompile(`(\p{Han})\s+「`)
	headingPattern        = regexp.MustCompile(`^[ \t]*#{1,6}[ \t]+(.*)$`)
	blockquotePattern     = regexp.MustCompile(`^[ \t]*>[ \t]?(.*)$`)
	listPattern           = regexp.MustCompile(`^([ \t]*)[-*+][ \t]+(.*)$`)
	rulePattern           = regexp.MustCompile(`^[ \t]*(?:-{3,}|\*{3,}|_{3,}|={3,})[ \t]*$`)
	tableSeparatorPattern = regexp.MustCompile(`^:?-+:?$`)
)

// RenderQQText converts common Markdown constructs to plain text for QQ
// ordinary messages. It runs at the QQ send boundary only; web, logs, database
// and API responses keep their original content.
func RenderQQText(message string) string {
	if strings.TrimSpace(message) == "" {
		return strings.TrimSpace(message)
	}
	codeBlocks := make([]string, 0)
	text := extractCodeBlocks(message, &codeBlocks)
	text = convertImageLinks(text)
	text = convertMarkdownLinks(text)
	text = convertInlineCode(text)
	text = convertEmphasis(text)
	text = convertMarkdownTables(text)

	lines := strings.Split(text, "\n")
	for index, line := range lines {
		lines[index] = convertPlainLine(line)
	}
	text = strings.Join(lines, "\n")
	text = cleanQQWhitespace(text)
	return restoreCodeBlocks(text, codeBlocks)
}

func extractCodeBlocks(text string, blocks *[]string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	inBlock := false
	content := make([]string, 0)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inBlock && strings.HasPrefix(trimmed, "```") {
			inBlock = true
			content = content[:0]
			continue
		}
		if inBlock && strings.HasPrefix(trimmed, "```") {
			inBlock = false
			*blocks = append(*blocks, strings.Join(content, "\n"))
			out = append(out, fmt.Sprintf(codeBlockMarker, len(*blocks)-1))
			continue
		}
		if inBlock {
			content = append(content, line)
			continue
		}
		out = append(out, line)
	}
	if inBlock {
		*blocks = append(*blocks, strings.Join(content, "\n"))
		out = append(out, fmt.Sprintf(codeBlockMarker, len(*blocks)-1))
	}
	return strings.Join(out, "\n")
}

func restoreCodeBlocks(text string, blocks []string) string {
	for index, content := range blocks {
		text = strings.ReplaceAll(text, fmt.Sprintf(codeBlockMarker, index), content)
	}
	return text
}

func convertImageLinks(text string) string {
	return imageLinkPattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := imageLinkPattern.FindStringSubmatch(match)
		alt := strings.TrimSpace(parts[1])
		url := strings.TrimSpace(parts[2])
		if alt == "" {
			return "图片\n" + url
		}
		return "图片：" + alt + "\n" + url
	})
}

func convertMarkdownLinks(text string) string {
	return markdownLinkPattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := markdownLinkPattern.FindStringSubmatch(match)
		label := strings.TrimSpace(parts[1])
		url := strings.TrimSpace(parts[2])
		if label == url {
			return url
		}
		return label + "：" + url
	})
}

func convertInlineCode(text string) string {
	text = inlineCodePattern.ReplaceAllString(text, "「$1」")
	return cjkSpacePattern.ReplaceAllString(text, "$1「")
}

func convertEmphasis(text string) string {
	for _, marker := range []string{"***", "**", "__", "*", "_"} {
		text = replacePairedMarkers(text, marker)
	}
	return text
}

func replacePairedMarkers(text, marker string) string {
	underscore := marker[0] == '_'
	var out strings.Builder
	index := 0
	for index < len(text) {
		openOffset := strings.Index(text[index:], marker)
		if openOffset < 0 {
			out.WriteString(text[index:])
			break
		}
		openPos := index + openOffset
		if openPos > 0 && skipOpeningBoundary(text[openPos-1], underscore) {
			out.WriteString(text[index : openPos+1])
			index = openPos + 1
			continue
		}
		searchFrom := openPos + len(marker)
		closeOffset := strings.Index(text[searchFrom:], marker)
		if closeOffset < 0 {
			out.WriteString(text[index:])
			break
		}
		closePos := searchFrom + closeOffset
		content := text[searchFrom:closePos]
		valid := strings.TrimSpace(content) != "" && !strings.HasPrefix(content, " ") && !strings.HasSuffix(content, " ") && !strings.Contains(content, "\n")
		if valid && closePos+len(marker) < len(text) && skipClosingBoundary(text[closePos+len(marker)], underscore) {
			valid = false
		}
		if !valid {
			out.WriteString(text[index : closePos+len(marker)])
			index = closePos + len(marker)
			continue
		}
		out.WriteString(text[index:openPos])
		out.WriteString(content)
		index = closePos + len(marker)
	}
	return out.String()
}

func skipOpeningBoundary(previous byte, underscore bool) bool {
	if underscore {
		return previous == '_' || isWordChar(previous)
	}
	return previous == '*'
}

func skipClosingBoundary(next byte, underscore bool) bool {
	if underscore {
		return next == '_' || isWordChar(next)
	}
	return next == '*'
}

func isWordChar(ch byte) bool {
	return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_'
}

func convertPlainLine(line string) string {
	if rulePattern.MatchString(line) {
		return "────────"
	}
	if matches := headingPattern.FindStringSubmatch(line); matches != nil {
		return "【" + strings.TrimSpace(matches[1]) + "】"
	}
	if matches := blockquotePattern.FindStringSubmatch(line); matches != nil {
		return matches[1]
	}
	if matches := listPattern.FindStringSubmatch(line); matches != nil {
		return matches[1] + "• " + matches[2]
	}
	return line
}

func convertMarkdownTables(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	index := 0
	for index < len(lines) {
		if isTableStart(lines, index) {
			header := splitTableRow(lines[index])
			rowIndex := index + 2
			rows := make([][]string, 0)
			for rowIndex < len(lines) && strings.Contains(strings.TrimSpace(lines[rowIndex]), "|") {
				rows = append(rows, splitTableRow(lines[rowIndex]))
				rowIndex++
			}
			out = append(out, renderTable(header, rows)...)
			index = rowIndex
			continue
		}
		out = append(out, lines[index])
		index++
	}
	return strings.Join(out, "\n")
}

func isTableStart(lines []string, index int) bool {
	if index+1 >= len(lines) {
		return false
	}
	if !strings.Contains(strings.TrimSpace(lines[index]), "|") {
		return false
	}
	separator := splitTableRow(lines[index+1])
	if len(separator) == 0 {
		return false
	}
	for _, cell := range separator {
		if !tableSeparatorPattern.MatchString(strings.TrimSpace(cell)) {
			return false
		}
	}
	return true
}

func splitTableRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	raw := strings.Split(line, "|")
	cells := make([]string, 0, len(raw))
	for _, cell := range raw {
		cells = append(cells, strings.TrimSpace(cell))
	}
	return cells
}

func renderTable(header []string, rows [][]string) []string {
	out := make([]string, 0, len(rows)+1)
	if len(header) > 0 && strings.TrimSpace(header[0]) != "" {
		out = append(out, "【"+strings.TrimSpace(header[0])+"】")
	}
	for _, row := range rows {
		out = append(out, strings.Join(row, "："))
	}
	return out
}

func cleanQQWhitespace(text string) string {
	lines := strings.Split(text, "\n")
	cleaned := make([]string, 0, len(lines))
	blankRun := 0
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			blankRun++
			if blankRun > 3 {
				continue
			}
		} else {
			blankRun = 0
		}
		cleaned = append(cleaned, line)
	}
	for len(cleaned) > 0 && strings.TrimSpace(cleaned[len(cleaned)-1]) == "" {
		cleaned = cleaned[:len(cleaned)-1]
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

// formatTextSegments returns a copy of the OneBot message segments with only
// text segment data rendered for QQ plain text. Other segment types are kept
// untouched so images, mentions, replies and similar segments survive intact.
func formatTextSegments(segments []jsonObject) []jsonObject {
	formatted := make([]jsonObject, len(segments))
	for index, segment := range segments {
		formatted[index] = segment
		if segment["type"] != "text" {
			continue
		}
		data := asObject(segment["data"])
		if raw, ok := data["text"].(string); ok {
			formatted[index] = jsonObject{
				"type": "text",
				"data": jsonObject{"text": RenderQQText(raw)},
			}
		}
	}
	return formatted
}
