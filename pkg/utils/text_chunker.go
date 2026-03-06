package utils

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

type TextChunkOptions struct {
	MaxChars     int
	OverlapChars int
	MinChars     int
	MaxChunks    int
}

type TextChunk struct {
	Index int
	Total int
	Text  string
}

func SplitLongText(text string, opts TextChunkOptions) []TextChunk {
	normalized := normalizeText(text)
	if strings.TrimSpace(normalized) == "" {
		return nil
	}

	if opts.MaxChars <= 0 {
		opts.MaxChars = 6000
	}
	if opts.OverlapChars < 0 {
		opts.OverlapChars = 0
	}
	if opts.MinChars <= 0 {
		opts.MinChars = minInt(opts.MaxChars/5, 1200)
	}
	if opts.MinChars > opts.MaxChars {
		opts.MinChars = opts.MaxChars
	}
	if opts.MaxChunks <= 0 {
		opts.MaxChunks = 12
	}

	if runeLen(normalized) <= opts.MaxChars {
		return []TextChunk{{Index: 1, Total: 1, Text: strings.TrimSpace(normalized)}}
	}

	segments := splitBySceneLikeHeadings(normalized)
	if len(segments) == 1 {
		segments = splitByParagraphsOrSentences(normalized, opts.MaxChars, opts.MinChars)
	}

	chunks := packSegmentsIntoChunks(segments, opts.MaxChars, opts.MinChars, opts.OverlapChars)
	chunks = capChunks(chunks, opts.MaxChunks, opts.MaxChars, opts.MinChars, opts.OverlapChars)

	total := len(chunks)
	out := make([]TextChunk, 0, total)
	for i, c := range chunks {
		out = append(out, TextChunk{
			Index: i + 1,
			Total: total,
			Text:  strings.TrimSpace(c),
		})
	}
	return out
}

func normalizeText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.TrimSpace(s)
}

func splitBySceneLikeHeadings(text string) []string {
	lines := strings.Split(text, "\n")
	re := regexp.MustCompile(`(?i)^\s*(第[一二三四五六七八九十0-9]+[场幕]|场景|scene\b|int\.|ext\.|INT\.|EXT\.|【场景|【Scene|#\s*scene)\b?`)

	var segments []string
	var buf strings.Builder
	flush := func() {
		seg := strings.TrimSpace(buf.String())
		if seg != "" {
			segments = append(segments, seg)
		}
		buf.Reset()
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && re.MatchString(trimmed) && buf.Len() > 0 {
			flush()
		}
		if buf.Len() > 0 {
			buf.WriteString("\n")
		}
		buf.WriteString(line)
	}
	flush()

	if len(segments) == 0 {
		return []string{text}
	}

	return segments
}

func splitByParagraphsOrSentences(text string, maxChars int, minChars int) []string {
	paras := splitByBlankLines(text)
	if len(paras) > 1 {
		return paras
	}

	return splitBySentenceBoundaries(text, maxChars, minChars)
}

func splitByBlankLines(text string) []string {
	raw := strings.Split(text, "\n\n")
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{text}
	}
	return out
}

func splitBySentenceBoundaries(text string, maxChars int, minChars int) []string {
	runes := []rune(text)
	n := len(runes)
	if n <= maxChars {
		return []string{text}
	}

	var segments []string
	start := 0
	for start < n {
		end := start + maxChars
		if end > n {
			end = n
		}

		cut := findBestCut(runes, start, end, minChars)
		segment := strings.TrimSpace(string(runes[start:cut]))
		segment = strings.Trim(segment, "\n")
		if segment != "" {
			segments = append(segments, segment)
		}
		start = cut
		for start < n && (runes[start] == '\n' || runes[start] == ' ' || runes[start] == '\t') {
			start++
		}
	}

	if len(segments) == 0 {
		return []string{text}
	}
	return segments
}

func findBestCut(runes []rune, start, end, minChars int) int {
	minEnd := start + minChars
	if minEnd > end {
		minEnd = end
	}

	best := -1
	for i := end - 1; i >= minEnd; i-- {
		switch runes[i] {
		case '。', '！', '？', '!', '?', '.', '；', ';':
			best = i + 1
			i = minEnd - 1
		}
	}
	if best != -1 {
		return best
	}

	for i := end - 1; i >= minEnd; i-- {
		if runes[i] == '\n' {
			return i + 1
		}
	}

	return end
}

func packSegmentsIntoChunks(segments []string, maxChars int, minChars int, overlapChars int) []string {
	var chunks []string
	var buf strings.Builder
	lastChunkTail := ""

	flush := func() {
		chunk := strings.TrimSpace(buf.String())
		if chunk == "" {
			buf.Reset()
			return
		}
		chunks = append(chunks, chunk)
		lastChunkTail = tailRunes(chunk, overlapChars)
		buf.Reset()
	}

	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}

		prefix := ""
		if buf.Len() == 0 && lastChunkTail != "" {
			prefix = lastChunkTail + "\n"
		}

		current := buf.String()
		next := current
		if next != "" {
			next += "\n\n"
		}
		next = prefix + next + seg

		if runeLen(next) <= maxChars {
			buf.Reset()
			buf.WriteString(next)
			continue
		}

		if buf.Len() > 0 && runeLen(current) >= minChars {
			flush()
			segWithPrefix := seg
			if lastChunkTail != "" {
				segWithPrefix = lastChunkTail + "\n" + seg
			}
			if runeLen(segWithPrefix) <= maxChars {
				buf.WriteString(segWithPrefix)
				continue
			}
		}

		longParts := splitBySentenceBoundaries(seg, maxChars, minChars)
		for _, part := range longParts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if buf.Len() == 0 && lastChunkTail != "" {
				part = lastChunkTail + "\n" + part
			}
			if runeLen(part) > maxChars {
				part = string([]rune(part)[:maxChars])
			}
			buf.WriteString(part)
			flush()
		}
	}

	flush()
	return chunks
}

func capChunks(chunks []string, maxChunks int, maxChars int, minChars int, overlapChars int) []string {
	if len(chunks) <= maxChunks {
		return chunks
	}

	merged := make([]string, 0, maxChunks)
	groupSize := (len(chunks) + maxChunks - 1) / maxChunks
	for i := 0; i < len(chunks); i += groupSize {
		end := i + groupSize
		if end > len(chunks) {
			end = len(chunks)
		}
		block := strings.Join(chunks[i:end], "\n\n")
		merged = append(merged, block)
	}

	if len(merged) <= maxChunks {
		if maxChars <= 0 {
			return merged
		}
		repacked := packSegmentsIntoChunks(merged, maxChars, minChars, overlapChars)
		if len(repacked) > 0 && len(repacked) <= maxChunks {
			return repacked
		}
		return merged[:maxChunks]
	}

	return merged[:maxChunks]
}

func tailRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[len(r)-n:])
}

func runeLen(s string) int {
	return utf8.RuneCountInString(s)
}
