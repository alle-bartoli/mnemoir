package memory

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/alle-bartoli/agentmem/internal/config"
	"github.com/alle-bartoli/agentmem/internal/embedding"
	goredis "github.com/redis/go-redis/v9"
)

const testProject = "test-store"

// newTestStore creates a Store backed by real Redis and local embedder.
// Requires Redis Stack running on localhost:6379.
func newTestStore(t *testing.T) *Store {
	t.Helper()

	rdb := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	emb, err := embedding.NewLocalEmbedder(config.EmbeddingLocalConfig{
		Model:    "sentence-transformers/all-MiniLM-L6-v2",
		ModelDir: "~/.agentmem/models",
	}, 384)
	if err != nil {
		t.Fatalf("NewLocalEmbedder: %v", err)
	}

	store := NewStore(rdb, emb)

	t.Cleanup(func() {
		// Clean up test keys
		keys, _ := rdb.Keys(ctx, "mem:test-*").Result()
		if len(keys) > 0 {
			rdb.Del(ctx, keys...)
		}
		rdb.SRem(ctx, "projects", testProject)
		_ = emb.Close()
		_ = rdb.Close()
	})

	return store
}

func newTestMemory(id, content string, memType MemoryType) *Memory {
	now := time.Now().Unix()
	return &Memory{
		ID:           id,
		Content:      content,
		Type:         memType,
		Project:      testProject,
		Tags:         "test",
		Importance:   5,
		SessionID:    "",
		CreatedAt:    now,
		LastAccessed: now,
		AccessCount:  0,
	}
}

func TestStore_SaveAndGet(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	mem := newTestMemory("test-save-get", "Redis runs on port 6379", Fact)

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
	if got.Type != Fact {
		t.Errorf("Type = %q, want %q", got.Type, Fact)
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
}

func TestStore_Delete(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	mem := newTestMemory("test-delete", "Temporary memory to delete", Narrative)

	if err := store.Save(ctx, mem); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify it exists
	_, err := store.Get(ctx, "test-delete")
	if err != nil {
		t.Fatalf("Get before delete: %v", err)
	}

	if err := store.Delete(ctx, "test-delete"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Get after delete should fail
	_, err = store.Get(ctx, "test-delete")
	if err == nil {
		t.Error("Get after delete should return error, got nil")
	}
}

func TestStore_DeleteNotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	err := store.Delete(ctx, "test-nonexistent-id")
	if err == nil {
		t.Error("Delete nonexistent should return error, got nil")
	}
}

func TestStore_GetNotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_, err := store.Get(ctx, "test-nonexistent-id")
	if err == nil {
		t.Error("Get nonexistent should return error, got nil")
	}
}

func TestStore_UpdateAccess(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	mem := newTestMemory("test-access", "Memory to access", Concept)

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
	// LastAccessed should be at least the original value (same second is ok)
	if got.LastAccessed < mem.LastAccessed {
		t.Errorf("LastAccessed = %d, should be >= %d", got.LastAccessed, mem.LastAccessed)
	}
}

func TestStore_Update(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	mem := newTestMemory("test-update", "Original content", Fact)

	if err := store.Save(ctx, mem); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Update content (triggers re-embedding) and importance
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
}

func TestStore_ListProjects(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	mem := newTestMemory("test-projects", "Project memory", Fact)

	if err := store.Save(ctx, mem); err != nil {
		t.Fatalf("Save: %v", err)
	}

	projects, err := store.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}

	found := slices.Contains(projects, testProject)
	if !found {
		t.Errorf("ListProjects did not include %q, got %v", testProject, projects)
	}
}
