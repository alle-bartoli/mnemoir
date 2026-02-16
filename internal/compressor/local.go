package compressor

import (
	"context"
	"regexp"
	"strings"
	"unicode"
)

// LocalCompressor extracts structured memories using rule-based text analysis.
// No external API or model required.
type LocalCompressor struct{}

// NewLocalCompressor creates a rule-based compressor.
func NewLocalCompressor() (*LocalCompressor, error) {
	return &LocalCompressor{}, nil
}

// Compress classifies sentences from observations into facts, concepts, and narratives.
func (c *LocalCompressor) Compress(_ context.Context, observations string) (*CompressResult, error) {
	sentences := splitSentences(observations)

	result := &CompressResult{
		Facts:      []ExtractedMemory{},
		Concepts:   []ExtractedMemory{},
		Narratives: []ExtractedMemory{},
	}

	for _, s := range sentences {
		s = strings.TrimSpace(s)
		if len(s) < 10 {
			continue
		}

		tags := extractTags(s)
		tagStr := strings.Join(tags, ",")

		switch classifySentence(s) {
		case "fact":
			result.Facts = append(result.Facts, ExtractedMemory{
				Content:    s,
				Tags:       tagStr,
				Importance: scoreFact(s),
			})
		case "concept":
			result.Concepts = append(result.Concepts, ExtractedMemory{
				Content:    s,
				Tags:       tagStr,
				Importance: scoreConcept(s),
			})
		default:
			result.Narratives = append(result.Narratives, ExtractedMemory{
				Content:    s,
				Tags:       tagStr,
				Importance: scoreNarrative(s),
			})
		}
	}

	return result, nil
}

// PRIVATE

var (
	// Fact patterns: file paths, ports, versions, URLs, IPs, env vars, config values
	reFilePath   = regexp.MustCompile(`(?:/[\w.-]+){2,}`)
	rePort       = regexp.MustCompile(`(?:port|PORT)\s*[:=]?\s*\d{2,5}|\b\d{2,5}/tcp\b`)
	reVersion    = regexp.MustCompile(`v?\d+\.\d+(?:\.\d+)`)
	reURL        = regexp.MustCompile(`https?://[^\s,)]+`)
	reIP         = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)
	reEnvVar     = regexp.MustCompile(`\b[A-Z][A-Z0-9_]{2,}\b`)
	reConfigKV   = regexp.MustCompile(`[\w.-]+\s*=\s*["']?[\w./:@-]+`)
	reWindowPath = regexp.MustCompile(`[A-Z]:\\[\w\\.-]+`)

	// Concept keywords
	conceptKeywords = []string{
		"pattern", "architecture", "design", "approach", "strategy",
		"principle", "convention", "paradigm", "framework", "methodology",
		"tradeoff", "trade-off", "abstraction", "interface", "protocol",
	}
)

func splitSentences(text string) []string {
	// Split on sentence terminators, bullet points, and newlines
	var sentences []string
	var current strings.Builder

	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if current.Len() > 0 {
				sentences = append(sentences, current.String())
				current.Reset()
			}
			continue
		}

		// Handle bullet/list items as separate sentences
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "• ") {
			if current.Len() > 0 {
				sentences = append(sentences, current.String())
				current.Reset()
			}
			sentences = append(sentences, strings.TrimLeft(line, "-*• "))
			continue
		}

		// Split on period followed by space and uppercase letter
		parts := splitOnSentenceEnd(line)
		for i, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if i < len(parts)-1 {
				if current.Len() > 0 {
					current.WriteString(" ")
				}
				current.WriteString(p)
				sentences = append(sentences, current.String())
				current.Reset()
			} else {
				if current.Len() > 0 {
					current.WriteString(" ")
				}
				current.WriteString(p)
			}
		}
	}

	if current.Len() > 0 {
		sentences = append(sentences, current.String())
	}

	return sentences
}

var reSentenceEnd = regexp.MustCompile(`\.\s+(?:[A-Z])`)

func splitOnSentenceEnd(text string) []string {
	indices := reSentenceEnd.FindAllStringIndex(text, -1)
	if len(indices) == 0 {
		return []string{text}
	}

	var parts []string
	start := 0
	for _, idx := range indices {
		// Split after the period, keep the uppercase letter for next part
		splitAt := idx[0] + 1 // include the period
		parts = append(parts, text[start:splitAt])
		start = splitAt + 1 // skip the space
	}
	if start < len(text) {
		parts = append(parts, text[start:])
	}
	return parts
}

func classifySentence(s string) string {
	lower := strings.ToLower(s)

	// Check fact patterns first (most specific)
	factScore := 0
	if reFilePath.MatchString(s) || reWindowPath.MatchString(s) {
		factScore++
	}
	if rePort.MatchString(s) {
		factScore++
	}
	if reVersion.MatchString(s) {
		factScore++
	}
	if reURL.MatchString(s) {
		factScore++
	}
	if reIP.MatchString(s) {
		factScore++
	}
	if reEnvVar.MatchString(s) {
		factScore++
	}
	if reConfigKV.MatchString(s) {
		factScore++
	}

	// Check concept keywords
	conceptScore := 0
	for _, kw := range conceptKeywords {
		if strings.Contains(lower, kw) {
			conceptScore++
		}
	}

	if factScore >= 2 {
		return "fact"
	}
	if conceptScore >= 1 {
		return "concept"
	}
	if factScore == 1 {
		return "fact"
	}
	return "narrative"
}

func scoreFact(s string) int {
	score := 6
	// More specific data points raise importance
	matches := 0
	if reFilePath.MatchString(s) || reWindowPath.MatchString(s) {
		matches++
	}
	if rePort.MatchString(s) {
		matches++
	}
	if reVersion.MatchString(s) {
		matches++
	}
	if reURL.MatchString(s) {
		matches++
	}
	if reIP.MatchString(s) {
		matches++
	}
	if matches >= 3 {
		score = 7
	}
	return score
}

func scoreConcept(s string) int {
	lower := strings.ToLower(s)
	score := 5
	hits := 0
	for _, kw := range conceptKeywords {
		if strings.Contains(lower, kw) {
			hits++
		}
	}
	if hits >= 2 {
		score = 6
	}
	return score
}

func scoreNarrative(_ string) int {
	return 4
}

func extractTags(s string) []string {
	seen := make(map[string]bool)
	var tags []string

	lower := strings.ToLower(s)

	// Extract technology/tool keywords
	techKeywords := []string{
		"redis", "docker", "go", "golang", "python", "node", "react",
		"postgres", "mysql", "mongodb", "api", "rest", "grpc", "graphql",
		"kubernetes", "k8s", "aws", "gcp", "azure", "linux", "nginx",
		"git", "ci", "cd", "test", "deploy", "config", "auth", "cache",
		"queue", "worker", "cron", "webhook", "websocket", "http", "tcp",
		"ssl", "tls", "dns", "cors", "jwt", "oauth", "oidc",
	}

	for _, kw := range techKeywords {
		if strings.Contains(lower, kw) && !seen[kw] {
			seen[kw] = true
			tags = append(tags, kw)
		}
	}

	// Extract concept keywords as tags
	for _, kw := range conceptKeywords {
		if strings.Contains(lower, kw) && !seen[kw] {
			seen[kw] = true
			tags = append(tags, kw)
		}
	}

	// Extract capitalized words as potential project/tool names (2-15 chars)
	words := strings.Fields(s)
	for _, w := range words {
		clean := strings.Trim(w, ".,;:!?()[]{}\"'")
		if len(clean) >= 2 && len(clean) <= 15 && isCapitalized(clean) && !seen[strings.ToLower(clean)] {
			tag := strings.ToLower(clean)
			seen[tag] = true
			tags = append(tags, tag)
		}
	}

	if len(tags) > 5 {
		tags = tags[:5]
	}

	return tags
}

func isCapitalized(s string) bool {
	if len(s) == 0 {
		return false
	}
	runes := []rune(s)
	if !unicode.IsUpper(runes[0]) {
		return false
	}
	// Must have at least one lowercase to avoid pure acronyms (caught by env var pattern)
	for _, r := range runes[1:] {
		if unicode.IsLower(r) {
			return true
		}
	}
	return false
}
