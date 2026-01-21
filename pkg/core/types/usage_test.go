package types

import (
	"testing"
)

func TestUsage_Add(t *testing.T) {
	u1 := Usage{
		InputTokens:  100,
		OutputTokens: 50,
		TotalTokens:  150,
	}
	u2 := Usage{
		InputTokens:  200,
		OutputTokens: 100,
		TotalTokens:  300,
	}

	result := u1.Add(u2)

	if result.InputTokens != 300 {
		t.Errorf("InputTokens = %d, want 300", result.InputTokens)
	}
	if result.OutputTokens != 150 {
		t.Errorf("OutputTokens = %d, want 150", result.OutputTokens)
	}
	if result.TotalTokens != 450 {
		t.Errorf("TotalTokens = %d, want 450", result.TotalTokens)
	}
}

func TestUsage_Add_WithOptionalFields(t *testing.T) {
	cacheRead := 10
	cacheWrite := 5
	cost := 0.01

	u1 := Usage{
		InputTokens:     100,
		OutputTokens:    50,
		TotalTokens:     150,
		CacheReadTokens: &cacheRead,
		CostUSD:         &cost,
	}
	u2 := Usage{
		InputTokens:      200,
		OutputTokens:     100,
		TotalTokens:      300,
		CacheWriteTokens: &cacheWrite,
		CostUSD:          &cost,
	}

	result := u1.Add(u2)

	if result.CacheReadTokens == nil || *result.CacheReadTokens != 10 {
		t.Errorf("CacheReadTokens = %v, want 10", result.CacheReadTokens)
	}
	if result.CacheWriteTokens == nil || *result.CacheWriteTokens != 5 {
		t.Errorf("CacheWriteTokens = %v, want 5", result.CacheWriteTokens)
	}
	if result.CostUSD == nil || *result.CostUSD != 0.02 {
		t.Errorf("CostUSD = %v, want 0.02", result.CostUSD)
	}
}

func TestUsage_IsEmpty(t *testing.T) {
	empty := Usage{}
	if !empty.IsEmpty() {
		t.Error("Empty usage should return true for IsEmpty()")
	}

	nonEmpty := Usage{InputTokens: 1}
	if nonEmpty.IsEmpty() {
		t.Error("Non-empty usage should return false for IsEmpty()")
	}
}
