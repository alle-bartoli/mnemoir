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
