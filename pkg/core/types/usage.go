package types

// Usage contains token counts and cost information.
type Usage struct {
	InputTokens      int      `json:"input_tokens"`
	OutputTokens     int      `json:"output_tokens"`
	TotalTokens      int      `json:"total_tokens"`
	CacheReadTokens  *int     `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens *int     `json:"cache_write_tokens,omitempty"`
	CostUSD          *float64 `json:"cost_usd,omitempty"`
}

// Add combines two Usage objects (for aggregation).
func (u Usage) Add(other Usage) Usage {
	result := Usage{
		InputTokens:  u.InputTokens + other.InputTokens,
		OutputTokens: u.OutputTokens + other.OutputTokens,
		TotalTokens:  u.TotalTokens + other.TotalTokens,
	}

	// Handle optional cache tokens
	if u.CacheReadTokens != nil || other.CacheReadTokens != nil {
		sum := 0
		if u.CacheReadTokens != nil {
			sum += *u.CacheReadTokens
		}
		if other.CacheReadTokens != nil {
			sum += *other.CacheReadTokens
		}
		result.CacheReadTokens = &sum
	}

	if u.CacheWriteTokens != nil || other.CacheWriteTokens != nil {
		sum := 0
		if u.CacheWriteTokens != nil {
			sum += *u.CacheWriteTokens
		}
		if other.CacheWriteTokens != nil {
			sum += *other.CacheWriteTokens
		}
		result.CacheWriteTokens = &sum
	}

	// Handle optional cost
	if u.CostUSD != nil || other.CostUSD != nil {
		var sum float64
		if u.CostUSD != nil {
			sum += *u.CostUSD
		}
		if other.CostUSD != nil {
			sum += *other.CostUSD
		}
		result.CostUSD = &sum
	}

	return result
}

// IsEmpty returns true if the usage has no tokens.
func (u Usage) IsEmpty() bool {
	return u.InputTokens == 0 && u.OutputTokens == 0 && u.TotalTokens == 0
}
