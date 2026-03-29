package memory

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"sort"
	"strconv"
	"time"
)

// VectorSearch performs KNN similarity search using embeddings.
// The query string is embedded into a vector, then RediSearch finds the K nearest
// neighbors via HNSW index using cosine distance.
//
// RediSearch returns cosine distance (0 = identical, 2 = opposite).
// Scores are later converted to similarity via: similarity = 1.0 - distance.
//
// The generated FT.SEARCH query:
//
//	FT.SEARCH idx:memories "(@project:{p})=>[KNN 10 @embedding $vec AS score]"
//	  PARAMS 2 vec <binary_blob>    -- query vector as little-endian float32 bytes
//	  SORTBY score                  -- lowest distance first (most similar)
//	  LIMIT 0 10
//	  DIALECT 2                     -- required for vector query syntax
func (s *Store) VectorSearch(ctx context.Context, query string, filters SearchFilters, limit int) ([]SearchResult, error) {
	results, err := s.vectorSearchRaw(ctx, query, filters, limit)
	if err != nil {
		return nil, err
	}
	s.trackAccess(ctx, results)
	return results, nil
}

// vectorSearchRaw performs KNN search without access tracking side effects.
// Used by HybridSearch to avoid double-counting access on merged results.
func (s *Store) vectorSearchRaw(ctx context.Context, query string, filters SearchFilters, limit int) ([]SearchResult, error) {
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

	return extractSearchResults(res)
}

// FullTextSearch performs text-based search using RediSearch FTS (Full-Text Search).
// RediSearch tokenizes the query and matches against the TEXT-indexed "content" field
// using TF-IDF scoring (Term Frequency - Inverse Document Frequency).
// WITHSCORES includes the relevance score in the response (higher = more relevant).
// Unlike VectorSearch, this finds exact/stemmed keyword matches, not semantic similarity.
func (s *Store) FullTextSearch(ctx context.Context, query string, filters SearchFilters, limit int) ([]SearchResult, error) {
	results, err := s.fullTextSearchRaw(ctx, query, filters, limit)
	if err != nil {
		return nil, err
	}
	s.trackAccess(ctx, results)
	return results, nil
}

// fullTextSearchRaw performs FTS without access tracking side effects.
// Used by HybridSearch to avoid double-counting access on merged results.
func (s *Store) fullTextSearchRaw(ctx context.Context, query string, filters SearchFilters, limit int) ([]SearchResult, error) {
	ftsQuery := buildFTSQuery(query, filters)

	res, err := s.rdb.Do(ctx,
		"FT.SEARCH", "idx:memories", ftsQuery,
		"WITHSCORES",
		"LIMIT", 0, limit,
	).Result()
	if err != nil {
		return nil, fmt.Errorf("fulltext search: %w", err)
	}

	return extractFTSResults(res)
}

// HybridSearch combines vector and fulltext search with weighted scoring.
// Runs both raw searches (no access tracking) with 2x the limit for better merge quality,
// merges and deduplicates by memory ID, then tracks access once per unique result.
//
// Default weights: 0.60 vector + 0.25 FTS + 0.15 importance (tunable via config).
func (s *Store) HybridSearch(ctx context.Context, query string, filters SearchFilters, limit int) ([]SearchResult, error) {
	fetchLimit := limit * 2

	// Use raw methods to avoid double access-count increment on overlapping results
	vectorResults, vecErr := s.vectorSearchRaw(ctx, query, filters, fetchLimit)
	ftsResults, ftsErr := s.fullTextSearchRaw(ctx, query, filters, fetchLimit)

	// Return whichever succeeds if one fails
	if vecErr != nil && ftsErr != nil {
		return nil, fmt.Errorf("both searches failed: vector=%w, fts=%v", vecErr, ftsErr)
	}
	if vecErr != nil {
		results := truncate(ftsResults, limit)
		s.trackAccess(ctx, results)
		return results, nil
	}
	if ftsErr != nil {
		results := truncate(vectorResults, limit)
		s.trackAccess(ctx, results)
		return results, nil
	}

	decayInterval, _ := s.memCfg.ParsedDecayInterval()
	merged := mergeResults(vectorResults, ftsResults,
		s.memCfg.VectorWeight, s.memCfg.FTSWeight, s.memCfg.ImportanceWeight,
		s.memCfg.DecayFactor, decayInterval,
		s.memCfg.AccessBoostFactor, s.memCfg.AccessBoostCap,
	)
	results := truncate(merged, limit)
	s.trackAccess(ctx, results)
	return results, nil
}

// PRIVATE

// trackAccess increments access_count for each unique result.
// Errors are logged but do not fail the search - stale access counts are acceptable.
func (s *Store) trackAccess(ctx context.Context, results []SearchResult) {
	for _, r := range results {
		if err := s.UpdateAccess(ctx, r.Memory.ID); err != nil {
			slog.Warn("trackAccess failed", "memory_id", r.Memory.ID, "error", err)
		}
	}
}

// @dev buildSearchFilter constructs a RediSearch filter expression for TAG fields.
// TAG filters use the syntax @field:{value} for exact match.
// Returns "*" (match all) if no filters are set.
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

// @dev buildFTSQuery builds a full-text search query string.
// User text is escaped to prevent RediSearch syntax injection (e.g. @, {, |).
// TAG filters are appended after the escaped text.
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

// @dev escapeQueryText prefixes RediSearch special characters with backslash.
// Without escaping, user input like "port:6379" would be parsed as field syntax.
func escapeQueryText(s string) string {
	// Security: escape all RediSearch special chars to prevent FTS query injection
	special := []byte{'@', '!', '{', '}', '(', ')', '|', '-', '=', '>', '[', ']', ':', ';', '~', '*', '\\', '"', '\'', '$', '#'}
	result := make([]byte, 0, len(s)*2)
	for i := 0; i < len(s); i++ {
		if slices.Contains(special, s[i]) {
			result = append(result, '\\')
		}
		result = append(result, s[i])
	}
	return string(result)
}

// @dev extractSearchResults parses FT.SEARCH response (used by VectorSearch).
// go-redis with RESP3 returns a map structure:
//
//	map[any]any{
//	  "total_results": int64,
//	  "results": []any{
//	    map[any]any{"id": "mem:ULID", "extra_attributes": map[any]any{...}, "score": float64},
//	  },
//	}
//
// The "score" from KNN is cosine distance (0=identical, 2=opposite).
// Converted to similarity: score = 1.0 - distance.
func extractSearchResults(res any) ([]SearchResult, error) {
	entries := getResultEntries(res)
	if len(entries) == 0 {
		return nil, nil
	}

	var results []SearchResult
	for _, entry := range entries {
		id := stripMemPrefix(getMapString(entry, "id"))
		vals := getExtraAttributes(entry)

		mem, err := hashToMemory(id, vals)
		if err != nil {
			continue
		}

		// Score from KNN is cosine distance (0=identical, 2=opposite)
		score := 0.0
		if s, ok := vals["score"]; ok {
			if d, err := strconv.ParseFloat(s, 64); err == nil {
				score = 1.0 - d
			}
		}

		results = append(results, SearchResult{Memory: *mem, Score: score})
	}

	return results, nil
}

// @dev extractFTSResults parses the FT.SEARCH WITHSCORES response into SearchResults.
// Same map structure as extractSearchResults, but the "score" field contains TF-IDF
// relevance (higher = more relevant) instead of cosine distance.
func extractFTSResults(res any) ([]SearchResult, error) {
	entries := getResultEntries(res)
	if len(entries) == 0 {
		return nil, nil
	}

	var results []SearchResult
	for _, entry := range entries {
		id := stripMemPrefix(getMapString(entry, "id"))
		vals := getExtraAttributes(entry)
		score := getMapFloat(entry, "score")

		mem, err := hashToMemory(id, vals)
		if err != nil {
			continue
		}

		results = append(results, SearchResult{Memory: *mem, Score: score})
	}

	return results, nil
}

// @dev mergeResults combines vector, full-text, and importance signals into a single ranked list.
// Both vec/FTS result sets use different score scales (cosine similarity vs TF-IDF), so each
// is normalized to [0, 1] by dividing by its max score. Importance is normalized by /10.
// Weighted scores are combined:
//
//	final_score = (vec_norm * vectorWeight) + (fts_norm * ftsWeight) + (imp_norm * importanceWeight)
//
// Default weights: 0.60 vector + 0.25 FTS + 0.15 importance.
func mergeResults(vectorResults, ftsResults []SearchResult,
	vectorWeight, ftsWeight, importanceWeight float64,
	decayFactor float64, decayInterval time.Duration,
	boostFactor, boostCap float64,
) []SearchResult {
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

	// Apply importance boost as a third scoring signal
	for id, r := range merged {
		effImp := r.Memory.EffectiveImportance(decayFactor, decayInterval, boostFactor, boostCap)
		impNormalized := effImp / 10.0 // normalize to [0, 1]
		r.Score += impNormalized * importanceWeight
		merged[id] = r
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

// @dev maxScore returns the highest score in a result set, used for normalization.
func maxScore(results []SearchResult) float64 {
	m := 0.0
	for _, r := range results {
		m = math.Max(m, r.Score)
	}
	return m
}

// @dev truncate caps the result slice to the requested limit.
func truncate(results []SearchResult, limit int) []SearchResult {
	if len(results) <= limit {
		return results
	}
	return results[:limit]
}
