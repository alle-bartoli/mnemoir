package memory

import (
	"math"
	"testing"
	"time"
)

const (
	testDecayFactor  = 0.9
	testBoostFactor  = 0.3
	testBoostCap     = 2.0
)

var testDecayInterval = 168 * time.Hour // 1 week

func newDecayMemory(importance int, weeksIdle int, accessCount int) *Memory {
	now := time.Now().Unix()
	lastAccessed := now - int64(weeksIdle)*7*24*3600
	return &Memory{
		Importance:   importance,
		LastAccessed: lastAccessed,
		AccessCount:  accessCount,
	}
}

func TestEffectiveImportance_NoDecay(t *testing.T) {
	mem := newDecayMemory(8, 0, 0)
	got := mem.EffectiveImportance(testDecayFactor, testDecayInterval, testBoostFactor, testBoostCap)

	if math.Abs(got-8.0) > 0.1 {
		t.Errorf("fresh memory: want ~8.0, got %.2f", got)
	}
}

func TestEffectiveImportance_AfterOneWeek(t *testing.T) {
	mem := newDecayMemory(8, 1, 0)
	got := mem.EffectiveImportance(testDecayFactor, testDecayInterval, testBoostFactor, testBoostCap)
	want := 8.0 * 0.9 // 7.2

	if math.Abs(got-want) > 0.1 {
		t.Errorf("one week idle: want ~%.1f, got %.2f", want, got)
	}
}

func TestEffectiveImportance_AfterThreeWeeks(t *testing.T) {
	mem := newDecayMemory(8, 3, 0)
	got := mem.EffectiveImportance(testDecayFactor, testDecayInterval, testBoostFactor, testBoostCap)
	want := 8.0 * math.Pow(0.9, 3) // ~5.832

	if math.Abs(got-want) > 0.1 {
		t.Errorf("three weeks idle: want ~%.1f, got %.2f", want, got)
	}
}

func TestEffectiveImportance_WithAccessBoost(t *testing.T) {
	// Same 3 weeks idle but 5 accesses: boost = min(2.0, 5*0.3) = 1.5
	memNoBoost := newDecayMemory(8, 3, 0)
	memBoosted := newDecayMemory(8, 3, 5)

	noBoost := memNoBoost.EffectiveImportance(testDecayFactor, testDecayInterval, testBoostFactor, testBoostCap)
	boosted := memBoosted.EffectiveImportance(testDecayFactor, testDecayInterval, testBoostFactor, testBoostCap)

	if boosted <= noBoost {
		t.Errorf("boosted (%.2f) should be > no boost (%.2f)", boosted, noBoost)
	}

	expectedBoost := 1.5
	diff := boosted - noBoost
	if math.Abs(diff-expectedBoost) > 0.1 {
		t.Errorf("boost diff: want ~%.1f, got %.2f", expectedBoost, diff)
	}
}

func TestEffectiveImportance_BoostCap(t *testing.T) {
	// 10 accesses: boost = min(2.0, 10*0.3) = min(2.0, 3.0) = 2.0
	mem7 := newDecayMemory(8, 3, 7)
	mem10 := newDecayMemory(8, 3, 10)

	eff7 := mem7.EffectiveImportance(testDecayFactor, testDecayInterval, testBoostFactor, testBoostCap)
	eff10 := mem10.EffectiveImportance(testDecayFactor, testDecayInterval, testBoostFactor, testBoostCap)

	// 7 accesses = min(2.0, 2.1) = 2.0 -> already capped
	// 10 accesses = min(2.0, 3.0) = 2.0 -> same cap
	if math.Abs(eff7-eff10) > 0.01 {
		t.Errorf("both should hit cap: 7acc=%.2f, 10acc=%.2f", eff7, eff10)
	}
}

func TestEffectiveImportance_FloorAtOne(t *testing.T) {
	mem := newDecayMemory(3, 20, 0)
	got := mem.EffectiveImportance(testDecayFactor, testDecayInterval, testBoostFactor, testBoostCap)

	// 3 * 0.9^20 = 3 * 0.1215 = 0.365 -> clamped to 1.0
	if math.Abs(got-1.0) > 0.01 {
		t.Errorf("floor: want 1.0, got %.2f", got)
	}
}
