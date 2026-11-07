package memory_test

import (
	"math"
	"testing"
	"time"

	"github.com/alle-bartoli/mnemoir/internal/memory"
)

// TestValidateTagValue tests the TAG injection prevention regex. No Redis needed.
func TestValidateTagValue(t *testing.T) {
	valid := []string{"redis", "my-tag", "v1.0", "go_lang", "test123"}
	for _, value := range valid {
		if err := memory.ValidateTagValue(value); err != nil {
			t.Errorf("ValidateTagValue(%q) should be valid, got: %v", value, err)
		}
	}

	invalid := []string{"foo|bar", "@inject", "a{b}", "", "a b", "tag;drop", "foo*", "a(b)"}
	for _, value := range invalid {
		if err := memory.ValidateTagValue(value); err == nil {
			t.Errorf("ValidateTagValue(%q) should be invalid, got nil", value)
		}
	}
}

// TestValidMemoryType tests type validation. No Redis needed.
func TestValidMemoryType(t *testing.T) {
	for _, value := range []string{string(memory.Fact), string(memory.Concept), string(memory.Narrative)} {
		if !memory.ValidMemoryType(value) {
			t.Errorf("ValidMemoryType(%q) should be true", value)
		}
	}
	for _, value := range []string{"", "invalid", "FACT", "Concept"} {
		if memory.ValidMemoryType(value) {
			t.Errorf("ValidMemoryType(%q) should be false", value)
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
