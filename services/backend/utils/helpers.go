package utils

import (
	"fmt"
	"strings"
)

// ChunkText splits text into chunks with overlap, optimized for semantic search
// Uses sentence boundaries when possible for better context preservation
func ChunkText(text string, size, overlap int) []string {
	if size <= 0 {
		return []string{text}
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= size {
		overlap = size / 2
	}

	var chunks []string
	textLen := len(text)

	for start := 0; start < textLen; {
		end := start + size
		if end > textLen {
			end = textLen
		}

		// Try to break at sentence boundary for better context
		if end < textLen {
			// Look for sentence endings within last 200 chars for better context
			searchStart := end - 200
			if searchStart < start {
				searchStart = start
			}
			substr := text[searchStart:end]
			// Приоритизируем более естественные границы предложений
			if idx := strings.LastIndexAny(substr, ".!?\n"); idx != -1 {
				end = searchStart + idx + 1
			} else if idx := strings.LastIndexAny(substr, ";,"); idx != -1 {
				// Если нет точки, ищем запятую или точку с запятой
				end = searchStart + idx + 1
			}
		}

		chunk := strings.TrimSpace(text[start:end])
		// Отфильтровываем слишком короткие чанки
		if chunk != "" && len(chunk) >= 50 {
			chunks = append(chunks, chunk)
		}

		if end >= textLen {
			break
		}

		start = end - overlap
		if start <= 0 {
			start = end
		}
	}

	return chunks
}

// ExtractRelevantTexts returns trimmed snippets for each document.
// It attempts to center the snippet around query keywords so we don't always take the start of the doc.
func ExtractRelevantTexts(docs []map[string]any, query string, maxChars int, window int) []string {
	out := make([]string, 0, len(docs))

	if maxChars <= 0 {
		maxChars = 0 // 0 means no per-document limit
	}
	if window <= 0 || window > maxChars {
		window = maxChars
	}

	queryLower := strings.ToLower(query)
	keywords := filterKeywords(queryLower)

	for _, d := range docs {
		txtRaw, ok := d["text"]
		if !ok {
			continue
		}
		text, ok := txtRaw.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}

		// If no limit, keep whole text
		if maxChars == 0 || len(text) <= maxChars {
			out = append(out, text)
			continue
		}

		// Try to find a keyword occurrence and center a window around it
		lowered := strings.ToLower(text)
		start := 0
		if len(keywords) > 0 {
			pos := findFirstKeyword(lowered, keywords)
			if pos >= 0 {
				half := window / 2
				start = pos - half
				if start < 0 {
					start = 0
				}
				if start+maxChars > len(text) {
					start = len(text) - maxChars
				}
			}
		}

		end := start + maxChars
		if end > len(text) {
			end = len(text)
		}
		snippet := strings.TrimSpace(text[start:end])
		if snippet != "" {
			out = append(out, snippet)
		}
	}

	return out
}

// filterKeywords keeps query tokens that are long enough to be meaningful
func filterKeywords(query string) []string {
	parts := strings.Fields(query)
	keywords := make([]string, 0, len(parts))
	for _, p := range parts {
		if len(p) >= 3 {
			keywords = append(keywords, p)
		}
	}
	return keywords
}

// findFirstKeyword returns the first occurrence index for any keyword (case-insensitive) or -1
func findFirstKeyword(text string, keywords []string) int {
	best := -1
	for _, kw := range keywords {
		if idx := strings.Index(text, kw); idx != -1 {
			if best == -1 || idx < best {
				best = idx
			}
		}
	}
	return best
}

// BuildContext creates a formatted context string from documents
func BuildContext(docs []string) string {
	if len(docs) == 0 {
		return ""
	}

	parts := make([]string, len(docs))
	for i, d := range docs {
		parts[i] = fmt.Sprintf("Document %d:\n%s", i+1, d)
	}

	return strings.Join(parts, "\n\n")
}

// SanitizeInput removes dangerous characters from user input
func SanitizeInput(input string) string {
	// Trim whitespace
	input = strings.TrimSpace(input)

	// Remove null bytes
	input = strings.ReplaceAll(input, "\x00", "")

	return input
}

// ValidateClientID checks if client ID is valid
func ValidateClientID(clientID string) error {
	if clientID == "" {
		return fmt.Errorf("client_id cannot be empty")
	}

	if len(clientID) > 255 {
		return fmt.Errorf("client_id too long (max 255 characters)")
	}

	// Check for dangerous characters
	if strings.ContainsAny(clientID, "\x00\n\r") {
		return fmt.Errorf("client_id contains invalid characters")
	}

	return nil
}

// ValidateQuery checks if query is valid
func ValidateQuery(query string) error {
	if query == "" {
		return fmt.Errorf("query cannot be empty")
	}

	if len(query) > 10000 {
		return fmt.Errorf("query too long (max 10000 characters)")
	}

	return nil
}
