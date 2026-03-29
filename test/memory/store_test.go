package memory_test

import (
	"context"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/alle-bartoli/agentmem/internal/memory"
)

func TestStore(t *testing.T) {
	t.Run("SaveAndGet", func(t *testing.T) {
		store := newTestStore(t)
		ctx := context.Background()

		mem := newTestMemory("test-save-get", "Redis runs on port 6379", memory.Fact)
		if err := store.Save(ctx, mem); err != nil {
			t.Fatalf("Save: %v", err)
		}

		got, err := store.Get(ctx, "test-save-get")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}

		if got.Content != mem.Content {
			t.Errorf("Content = %q, want %q", got.Content, mem.Content)
		}
		if got.Type != memory.Fact {
			t.Errorf("Type = %q, want %q", got.Type, memory.Fact)
		}
		if got.Project != testProject {
			t.Errorf("Project = %q, want %q", got.Project, testProject)
		}
		if got.Importance != 5 {
			t.Errorf("Importance = %d, want 5", got.Importance)
		}
		if got.Tags != "test" {
			t.Errorf("Tags = %q, want %q", got.Tags, "test")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		store := newTestStore(t)
		ctx := context.Background()

		mem := newTestMemory("test-delete", "Temporary memory to delete", memory.Narrative)
		if err := store.Save(ctx, mem); err != nil {
			t.Fatalf("Save: %v", err)
		}

		if _, err := store.Get(ctx, "test-delete"); err != nil {
			t.Fatalf("Get before delete: %v", err)
		}

		if err := store.Delete(ctx, "test-delete"); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		if _, err := store.Get(ctx, "test-delete"); err == nil {
			t.Error("Get after delete should return error, got nil")
		}
	})

	t.Run("DeleteNotFound", func(t *testing.T) {
		store := newTestStore(t)
		ctx := context.Background()

		if err := store.Delete(ctx, "test-nonexistent-id"); err == nil {
			t.Error("Delete nonexistent should return error, got nil")
		}
	})

	t.Run("GetNotFound", func(t *testing.T) {
		store := newTestStore(t)
		ctx := context.Background()

		if _, err := store.Get(ctx, "test-nonexistent-id"); err == nil {
			t.Error("Get nonexistent should return error, got nil")
		}
	})

	t.Run("UpdateAccess", func(t *testing.T) {
		store := newTestStore(t)
		ctx := context.Background()

		mem := newTestMemory("test-access", "Memory to access", memory.Concept)
		if err := store.Save(ctx, mem); err != nil {
			t.Fatalf("Save: %v", err)
		}

		if err := store.UpdateAccess(ctx, "test-access"); err != nil {
			t.Fatalf("UpdateAccess: %v", err)
		}

		got, err := store.Get(ctx, "test-access")
		if err != nil {
			t.Fatalf("Get after access: %v", err)
		}

		if got.AccessCount != 1 {
			t.Errorf("AccessCount = %d, want 1", got.AccessCount)
		}
		if got.LastAccessed < mem.LastAccessed {
			t.Errorf("LastAccessed = %d, should be >= %d", got.LastAccessed, mem.LastAccessed)
		}
	})

	t.Run("Update", func(t *testing.T) {
		store := newTestStore(t)
		ctx := context.Background()

		mem := newTestMemory("test-update", "Original content", memory.Fact)
		if err := store.Save(ctx, mem); err != nil {
			t.Fatalf("Save: %v", err)
		}

		err := store.Update(ctx, "test-update", map[string]any{
			"content":    "Updated content with new info",
			"importance": 8,
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}

		got, err := store.Get(ctx, "test-update")
		if err != nil {
			t.Fatalf("Get after update: %v", err)
		}

		if got.Content != "Updated content with new info" {
			t.Errorf("Content = %q, want %q", got.Content, "Updated content with new info")
		}
	})

	t.Run("ListProjects", func(t *testing.T) {
		store := newTestStore(t)
		ctx := context.Background()

		mem := newTestMemory("test-projects", "Project memory", memory.Fact)
		if err := store.Save(ctx, mem); err != nil {
			t.Fatalf("Save: %v", err)
		}

		projects, err := store.ListProjects(ctx)
		if err != nil {
			t.Fatalf("ListProjects: %v", err)
		}

		if !slices.Contains(projects, testProject) {
			t.Errorf("ListProjects did not include %q, got %v", testProject, projects)
		}
	})

	t.Run("SessionSaveAndGetLast", func(t *testing.T) {
		store := newTestStore(t)
		ctx := context.Background()

		sess1 := &memory.Session{
			ID: "test-sess-001", Project: testProject,
			StartedAt: time.Now().Add(-2 * time.Hour).Unix(),
			Summary:   "first session",
		}
		sess2 := &memory.Session{
			ID: "test-sess-002", Project: testProject,
			StartedAt: time.Now().Unix(),
			Summary:   "second session",
		}

		if err := store.SaveSession(ctx, sess1); err != nil {
			t.Fatalf("SaveSession 1: %v", err)
		}
		if err := store.SaveSession(ctx, sess2); err != nil {
			t.Fatalf("SaveSession 2: %v", err)
		}

		last, err := store.GetLastSession(ctx, testProject)
		if err != nil {
			t.Fatalf("GetLastSession: %v", err)
		}
		if last == nil {
			t.Fatal("GetLastSession returned nil")
		}
		if last.ID != "test-sess-002" {
			t.Errorf("GetLastSession ID = %q, want test-sess-002", last.ID)
		}
		if last.Summary != "second session" {
			t.Errorf("Summary = %q, want 'second session'", last.Summary)
		}
	})

	t.Run("GetLastSessionNoSessions", func(t *testing.T) {
		store := newTestStore(t)
		ctx := context.Background()

		last, err := store.GetLastSession(ctx, "nonexistent-project")
		if err != nil {
			t.Fatalf("GetLastSession: %v", err)
		}
		if last != nil {
			t.Errorf("expected nil for project with no sessions, got %+v", last)
		}
	})

	t.Run("GetStats", func(t *testing.T) {
		store, _ := newSearchTestStore(t)
		ctx := context.Background()

		stats, err := store.GetStats(ctx, searchTestProject)
		if err != nil {
			t.Fatalf("GetStats: %v", err)
		}

		// testMemories() creates 8 memories: 4 facts, 3 concepts, 1 narrative
		if stats.Total < 8 {
			t.Errorf("Total = %d, want >= 8", stats.Total)
		}
		if stats.ByType["fact"] < 4 {
			t.Errorf("ByType[fact] = %d, want >= 4", stats.ByType["fact"])
		}
		if stats.ByType["concept"] < 3 {
			t.Errorf("ByType[concept] = %d, want >= 3", stats.ByType["concept"])
		}
		if stats.AvgImportance == 0 {
			t.Error("AvgImportance should be > 0")
		}
		t.Logf("Stats: total=%d, by_type=%v, avg_imp=%.2f", stats.Total, stats.ByType, stats.AvgImportance)
	})

	t.Run("GetTopMemories", func(t *testing.T) {
		store, _ := newSearchTestStore(t)
		ctx := context.Background()

		top, err := store.GetTopMemories(ctx, searchTestProject, 3)
		if err != nil {
			t.Fatalf("GetTopMemories: %v", err)
		}
		if len(top) == 0 {
			t.Fatal("GetTopMemories returned 0 results")
		}
		if len(top) > 3 {
			t.Errorf("GetTopMemories returned %d results, want <= 3", len(top))
		}

		// Results should be sorted by importance DESC
		for i := 1; i < len(top); i++ {
			if top[i].Importance > top[i-1].Importance {
				t.Errorf("not sorted by importance DESC: [%d]=%d > [%d]=%d",
					i, top[i].Importance, i-1, top[i-1].Importance)
			}
		}
		t.Logf("Top memories: %d returned, first importance=%d", len(top), top[0].Importance)
	})
}

// TestValidateTagValue tests the TAG injection prevention regex. No Redis needed.
func TestValidateTagValue(t *testing.T) {
	valid := []string{"redis", "my-tag", "v1.0", "go_lang", "test123"}
	for _, v := range valid {
		if err := memory.ValidateTagValue(v); err != nil {
			t.Errorf("ValidateTagValue(%q) should be valid, got: %v", v, err)
		}
	}

	invalid := []string{"foo|bar", "@inject", "a{b}", "", "a b", "tag;drop", "foo*", "a(b)"}
	for _, v := range invalid {
		if err := memory.ValidateTagValue(v); err == nil {
			t.Errorf("ValidateTagValue(%q) should be invalid, got nil", v)
		}
	}
}

// TestValidMemoryType tests type validation. No Redis needed.
func TestValidMemoryType(t *testing.T) {
	for _, v := range []string{"fact", "concept", "narrative"} {
		if !memory.ValidMemoryType(v) {
			t.Errorf("ValidMemoryType(%q) should be true", v)
		}
	}
	for _, v := range []string{"", "invalid", "FACT", "Concept"} {
		if memory.ValidMemoryType(v) {
			t.Errorf("ValidMemoryType(%q) should be false", v)
		}
	}
}

// TestEffectiveImportance groups pure unit tests for the decay/boost formula.
// No Redis needed.
func TestEffectiveImportance(t *testing.T) {
	const (
		decayFactor = 0.9
		boostFactor = 0.3
		boostCap    = 2.0
	)
	decayInterval := 168 * time.Hour

	newMem := func(importance int, weeksIdle int, accessCount int) *memory.Memory {
		now := time.Now().Unix()
		return &memory.Memory{
			Importance:   importance,
			LastAccessed: now - int64(weeksIdle)*7*24*3600,
			AccessCount:  accessCount,
		}
	}

	t.Run("NoDecay", func(t *testing.T) {
		got := newMem(8, 0, 0).EffectiveImportance(decayFactor, decayInterval, boostFactor, boostCap)
		if math.Abs(got-8.0) > 0.1 {
			t.Errorf("fresh memory: want ~8.0, got %.2f", got)
		}
	})

	t.Run("AfterOneWeek", func(t *testing.T) {
		got := newMem(8, 1, 0).EffectiveImportance(decayFactor, decayInterval, boostFactor, boostCap)
		want := 8.0 * 0.9
		if math.Abs(got-want) > 0.1 {
			t.Errorf("one week idle: want ~%.1f, got %.2f", want, got)
		}
	})

	t.Run("AfterThreeWeeks", func(t *testing.T) {
		got := newMem(8, 3, 0).EffectiveImportance(decayFactor, decayInterval, boostFactor, boostCap)
		want := 8.0 * math.Pow(0.9, 3)
		if math.Abs(got-want) > 0.1 {
			t.Errorf("three weeks idle: want ~%.1f, got %.2f", want, got)
		}
	})

	t.Run("WithAccessBoost", func(t *testing.T) {
		noBoost := newMem(8, 3, 0).EffectiveImportance(decayFactor, decayInterval, boostFactor, boostCap)
		boosted := newMem(8, 3, 5).EffectiveImportance(decayFactor, decayInterval, boostFactor, boostCap)

		if boosted <= noBoost {
			t.Errorf("boosted (%.2f) should be > no boost (%.2f)", boosted, noBoost)
		}
		if diff := boosted - noBoost; math.Abs(diff-1.5) > 0.1 {
			t.Errorf("boost diff: want ~1.5, got %.2f", diff)
		}
	})

	t.Run("BoostCap", func(t *testing.T) {
		eff7 := newMem(8, 3, 7).EffectiveImportance(decayFactor, decayInterval, boostFactor, boostCap)
		eff10 := newMem(8, 3, 10).EffectiveImportance(decayFactor, decayInterval, boostFactor, boostCap)
		if math.Abs(eff7-eff10) > 0.01 {
			t.Errorf("both should hit cap: 7acc=%.2f, 10acc=%.2f", eff7, eff10)
		}
	})

	t.Run("FloorAtOne", func(t *testing.T) {
		got := newMem(3, 20, 0).EffectiveImportance(decayFactor, decayInterval, boostFactor, boostCap)
		if math.Abs(got-1.0) > 0.01 {
			t.Errorf("floor: want 1.0, got %.2f", got)
		}
	})
}
