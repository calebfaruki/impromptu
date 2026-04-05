package index

import (
	"context"
	"fmt"
	"strings"
)

// SearchPrompts performs full-text search across prompt names, descriptions, and authors.
// Results are ranked by BM25 with name weighted 10x, author 5x, description 1x.
func (d *DB) SearchPrompts(ctx context.Context, query string, limit, offset int) ([]SearchResult, error) {
	if query == "" {
		return []SearchResult{}, nil
	}
	if limit <= 0 {
		limit = 20
	}

	sanitized := sanitizeQuery(query)
	if sanitized == "" {
		return []SearchResult{}, nil
	}

	rows, err := d.db.QueryContext(ctx, `
		SELECT p.id, p.name, p.description, a.username, a.display_name,
		       bm25(prompts_fts, 10.0, 1.0, 5.0) AS rank
		FROM prompts_fts
		JOIN prompts p ON p.rowid = prompts_fts.rowid
		JOIN authors a ON a.id = p.author_id
		WHERE prompts_fts MATCH ?
		ORDER BY rank
		LIMIT ? OFFSET ?`,
		sanitized, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("searching prompts for %q: %w", query, err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.PromptID, &r.Name, &r.Description, &r.Author, &r.DisplayName, &r.Rank); err != nil {
			return nil, fmt.Errorf("scanning search result: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating search results: %w", err)
	}
	return results, nil
}

// sanitizeQuery wraps each word in double quotes to prevent FTS5 syntax errors.
func sanitizeQuery(q string) string {
	words := strings.Fields(q)
	var out []string
	for _, w := range words {
		w = strings.ReplaceAll(w, "\"", "")
		if w != "" {
			out = append(out, "\""+w+"\"")
		}
	}
	return strings.Join(out, " ")
}
