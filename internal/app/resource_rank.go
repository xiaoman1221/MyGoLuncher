package app

import (
	"sort"
	"strings"
	"unicode"
)

// compactSearchText removes punctuation and spacing so "Farmer's Delight",
// "farmers-delight" and "farmers delight" compare as the same phrase.
func compactSearchText(value string) string {
	var builder strings.Builder
	for _, char := range strings.ToLower(value) {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

func searchWords(value string) []string {
	fields := strings.FieldsFunc(strings.ToLower(value), func(char rune) bool {
		return !unicode.IsLetter(char) && !unicode.IsDigit(char)
	})
	words := make([]string, 0, len(fields))
	for _, field := range fields {
		if field != "" {
			words = append(words, field)
		}
	}
	return words
}

func projectMatchScore(query string, project ResourceProject) int {
	query = strings.TrimSpace(query)
	if query == "" {
		return 0
	}
	compactQuery := compactSearchText(query)
	if compactQuery == "" {
		return 0
	}
	title := compactSearchText(project.Title)
	slug := compactSearchText(project.Slug)
	switch {
	case title == compactQuery, slug == compactQuery:
		return 0
	case strings.HasPrefix(title, compactQuery), strings.HasPrefix(slug, compactQuery):
		return 1
	case strings.Contains(title, compactQuery), strings.Contains(slug, compactQuery):
		return 2
	}

	// Word coverage handles word-order differences such as "JEI Just Enough
	// Items" vs "Just Enough Items (JEI)".
	words := searchWords(query)
	if len(words) > 0 {
		haystack := title + " " + slug
		matched := 0
		for _, word := range words {
			if strings.Contains(haystack, word) {
				matched++
			}
		}
		if matched == len(words) {
			return 3
		}
		if matched*4 >= len(words)*3 {
			return 4
		}
	}
	return 5
}

// rankResourceProjects puts an exact/near-exact project name above add-ons and
// compatibility patches. It intentionally uses stable sorting afterwards by
// downloads so the ranking does not feel random for generic queries.
func rankResourceProjects(projects []ResourceProject, query string) {
	if len(projects) < 2 || strings.TrimSpace(query) == "" {
		return
	}
	sort.SliceStable(projects, func(i, j int) bool {
		left := projectMatchScore(query, projects[i])
		right := projectMatchScore(query, projects[j])
		if left != right {
			return left < right
		}
		if projects[i].Downloads != projects[j].Downloads {
			return projects[i].Downloads > projects[j].Downloads
		}
		return strings.ToLower(projects[i].Title) < strings.ToLower(projects[j].Title)
	})
}
