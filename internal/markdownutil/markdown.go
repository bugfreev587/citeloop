package markdownutil

import "strings"

var generatedMarkdownEscapeReplacer = strings.NewReplacer(
	"\\#", "#",
	"\\*", "*",
	"\\-", "-",
	"\\+", "+",
	"\\>", ">",
	"\\_", "_",
	"\\[", "[",
	"\\]", "]",
	"\\(", "(",
	"\\)", ")",
	"\\|", "|",
	"\\`", "`",
	"\\!", "!",
)

// NormalizeGeneratedEscapes restores Markdown control characters that LLMs
// sometimes defensively escape in article bodies.
func NormalizeGeneratedEscapes(s string) string {
	if s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	inFence := false
	fenceMarker := ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if inFence {
			lines[i] = line
			if fenceMarker != "" && strings.HasPrefix(trimmed, fenceMarker) {
				inFence = false
				fenceMarker = ""
			}
			continue
		}

		normalized := generatedMarkdownEscapeReplacer.Replace(line)
		lines[i] = normalized
		switch trimmed = strings.TrimSpace(normalized); {
		case strings.HasPrefix(trimmed, "```"):
			inFence = true
			fenceMarker = "```"
		case strings.HasPrefix(trimmed, "~~~"):
			inFence = true
			fenceMarker = "~~~"
		}
	}
	return strings.Join(lines, "\n")
}
