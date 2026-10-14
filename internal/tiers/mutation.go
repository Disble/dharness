package tiers

// AllowsMutation preserves legacy scope and excludes only declared infrastructure.
// Coverage targets never become mutation-score thresholds.
func (r Resolution) AllowsMutation(path string) bool {
	tier, _ := r.Tier(path)
	return tier != "infrastructure"
}
