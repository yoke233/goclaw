package memory

import (
	"strings"
	"testing"
)

func TestSearchVectorFilterSQLShouldNotContainDoubleParentheses(t *testing.T) {
	opts := SearchOptions{
		Sources: []MemorySource{MemorySourceSession, MemorySourceDaily},
		Types:   []MemoryType{MemoryTypeFact, MemoryTypeContext},
	}

	querySQL := `
		SELECT m.id
		FROM memory_vec v
		JOIN memories m ON m.id = v.id
		WHERE m.source IN (` + sourcePlaceholders(opts.Sources) + `)
		AND m.type IN (` + typePlaceholders(opts.Types) + `)
	`

	if strings.Contains(querySQL, "IN ((") || strings.Contains(querySQL, "))") {
		t.Fatalf("unexpected nested parentheses in SQL filters: %s", querySQL)
	}
}
