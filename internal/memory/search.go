package memory

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
)

// VectorSearch performs KNN similarity search using embeddings.
func (s *Store) VectorSearch(ctx context.Context, query string, filters SearchFilters, limit int) ([]SearchResult, error) {
	emb, err := s.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	filterExpr := buildSearchFilter(filters)
	blob := float32ToBytes(emb)

	args := []any{
		"FT.SEARCH", "idx:memories",
		fmt.Sprintf("(%s)=>[KNN %d @embedding $vec AS score]", filterExpr, limit),
		"PARAMS", "2", "vec", blob,
		"SORTBY", "score",
		"LIMIT", 0, limit,
		"DIALECT", "2",
	}

	res, err := s.rdb.Do(ctx, args...).Result()
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}

	results, err := extractSearchResults(res)
	if err != nil {
		return nil, err
	}

	// Update access tracking for returned results
	for _, r := range results {
		_ = s.UpdateAccess(ctx, r.Memory.ID)
	}

	return results, nil
}

// FullTextSearch performs text-based search using RediSearch FTS.
func (s *Store) FullTextSearch(ctx context.Context, query string, filters SearchFilters, limit int) ([]SearchResult, error) {
	ftsQuery := buildFTSQuery(query, filters)

	res, err := s.rdb.Do(ctx,
		"FT.SEARCH", "idx:memories", ftsQuery,
		"WITHSCORES",
		"LIMIT", 0, limit,
	).Result()
	if err != nil {
		return nil, fmt.Errorf("fulltext search: %w", err)
	}

	results, err := extractFTSResults(res)
	if err != nil {
		return nil, err
	}

	for _, r := range results {
		_ = s.UpdateAccess(ctx, r.Memory.ID)
	}

	return results, nil
}

// HybridSearch combines vector and fulltext search with weighted scoring.
func (s *Store) HybridSearch(ctx context.Context, query string, filters SearchFilters, limit int) ([]SearchResult, error) {
	// Run both searches with a larger limit for better merge quality
	fetchLimit := limit * 2

	vectorResults, vecErr := s.VectorSearch(ctx, query, filters, fetchLimit)
	ftsResults, ftsErr := s.FullTextSearch(ctx, query, filters, fetchLimit)

	// Return whichever succeeds if one fails
	if vecErr != nil && ftsErr != nil {
		return nil, fmt.Errorf("both searches failed: vector=%w, fts=%v", vecErr, ftsErr)
	}
	if vecErr != nil {
		return truncate(ftsResults, limit), nil
	}
	if ftsErr != nil {
		return truncate(vectorResults, limit), nil
	}

	merged := mergeResults(vectorResults, ftsResults, 0.7, 0.3)
	return truncate(merged, limit), nil
}

// PRIVATE

func buildSearchFilter(filters SearchFilters) string {
	parts := []string{}
	if filters.Project != "" {
		parts = append(parts, fmt.Sprintf("@project:{%s}", escapeTag(filters.Project)))
	}
	if filters.Type != "" {
		parts = append(parts, fmt.Sprintf("@type:{%s}", escapeTag(filters.Type)))
	}
	if len(parts) == 0 {
		return "*"
	}
	query := ""
	for i, p := range parts {
		if i > 0 {
			query += " "
		}
		query += p
	}
	return query
}

func buildFTSQuery(text string, filters SearchFilters) string {
	// Escape special RediSearch query characters in user text
	escaped := escapeQueryText(text)
	query := escaped

	if filters.Project != "" {
		query += fmt.Sprintf(" @project:{%s}", escapeTag(filters.Project))
	}
	if filters.Type != "" {
		query += fmt.Sprintf(" @type:{%s}", escapeTag(filters.Type))
	}
	return query
}

func escapeQueryText(s string) string {
	special := []byte{'@', '!', '{', '}', '(', ')', '|', '-', '=', '>', '[', ']', ':', ';', '~', '*'}
	result := make([]byte, 0, len(s)*2)
	for i := 0; i < len(s); i++ {
		if slices.Contains(special, s[i]) {
			result = append(result, '\\')
		}
		result = append(result, s[i])
	}
	return string(result)
}

func extractSearchResults(res any) ([]SearchResult, error) {
	arr, ok := res.([]any)
	if !ok || len(arr) < 1 {
		return nil, nil
	}

	var results []SearchResult
	// Format: [total, key1, [field1, val1, ...], key2, ...]
	for i := 1; i+1 < len(arr); i += 2 {
		key, ok := arr[i].(string)
		if !ok {
			continue
		}
		fieldArr, ok := arr[i+1].([]any)
		if !ok {
			continue
		}

		vals := arrayToMap(fieldArr)
		id := key
		if len(key) > 4 {
			id = key[4:]
		}

		mem, err := hashToMemory(id, vals)
		if err != nil {
			continue
		}

		// Score from KNN is cosine distance (0=identical, 2=opposite)
		// Convert to similarity: 1 - distance
		score := 0.0
		if s, err := strconv.ParseFloat(vals["score"], 64); err == nil {
			score = 1.0 - s
		}

		results = append(results, SearchResult{Memory: *mem, Score: score})
	}

	return results, nil
}

func extractFTSResults(res any) ([]SearchResult, error) {
	arr, ok := res.([]any)
	if !ok || len(arr) < 1 {
		return nil, nil
	}

	var results []SearchResult
	// With WITHSCORES: [total, key1, score1, [fields...], key2, score2, [fields...], ...]
	for i := 1; i+2 < len(arr); i += 3 {
		key, ok := arr[i].(string)
		if !ok {
			continue
		}

		scoreStr, ok := arr[i+1].(string)
		if !ok {
			continue
		}
		score, _ := strconv.ParseFloat(scoreStr, 64)

		fieldArr, ok := arr[i+2].([]any)
		if !ok {
			continue
		}

		vals := arrayToMap(fieldArr)
		id := key
		if len(key) > 4 {
			id = key[4:]
		}

		mem, err := hashToMemory(id, vals)
		if err != nil {
			continue
		}

		results = append(results, SearchResult{Memory: *mem, Score: score})
	}

	return results, nil
}

func mergeResults(vectorResults, ftsResults []SearchResult, vectorWeight, ftsWeight float64) []SearchResult {
	merged := make(map[string]SearchResult)

	// Normalize vector scores
	vecMax := maxScore(vectorResults)
	for _, r := range vectorResults {
		normalized := 0.0
		if vecMax > 0 {
			normalized = r.Score / vecMax
		}
		merged[r.Memory.ID] = SearchResult{
			Memory: r.Memory,
			Score:  normalized * vectorWeight,
		}
	}

	// Normalize FTS scores and merge
	ftsMax := maxScore(ftsResults)
	for _, r := range ftsResults {
		normalized := 0.0
		if ftsMax > 0 {
			normalized = r.Score / ftsMax
		}
		if existing, ok := merged[r.Memory.ID]; ok {
			existing.Score += normalized * ftsWeight
			merged[r.Memory.ID] = existing
		} else {
			merged[r.Memory.ID] = SearchResult{
				Memory: r.Memory,
				Score:  normalized * ftsWeight,
			}
		}
	}

	results := make([]SearchResult, 0, len(merged))
	for _, r := range merged {
		results = append(results, r)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}

func maxScore(results []SearchResult) float64 {
	m := 0.0
	for _, r := range results {
		m = math.Max(m, r.Score)
	}
	return m
}

func truncate(results []SearchResult, limit int) []SearchResult {
	if len(results) <= limit {
		return results
	}
	return results[:limit]
}
