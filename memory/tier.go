package memory

import (
	"time"
)

// MigrateEntries performs tier migration on a slice of MemoryEntry.
// It applies downgrade rules based on last access time and returns
// a list of entries that need updating, plus counts of pruned IDs.
//
// Migration rules:
//   - HOT → WARM: LastAccess older than hotWarmDays AND AccessCount < minAccess
//   - WARM → COLD: LastAccess older than warmColdDays
//   - COLD → prune: LastAccess older than coldPruneDays
//   - TierEternal entries are never migrated or pruned.
//
// The caller is responsible for persisting the returned entries
// and deleting the pruned IDs.
func MigrateEntries(
	entries []MemoryEntry,
	hotWarmDays int,
	warmColdDays int,
	coldPruneDays int,
	minAccessCount int,
) (updated []MemoryEntry, prunedIDs []string) {
	now := time.Now()
	hotCutoff := now.Add(-time.Duration(hotWarmDays) * 24 * time.Hour)
	warmCutoff := now.Add(-time.Duration(warmColdDays) * 24 * time.Hour)
	coldCutoff := now.Add(-time.Duration(coldPruneDays) * 24 * time.Hour)

	for _, e := range entries {
		if e.Tier == TierEternal {
			continue
		}

		switch e.Tier {
		case TierHot, "":
			// "" (unset) is treated as HOT for backward compatibility.
			if e.AccessCount >= int64(minAccessCount) && e.LastAccess.After(hotCutoff) {
				continue
			}
			e.Tier = TierWarm
			updated = append(updated, e)

		case TierWarm:
			if e.LastAccess.After(warmCutoff) {
				continue
			}
			e.Tier = TierCold
			updated = append(updated, e)

		case TierCold:
			if e.LastAccess.After(coldCutoff) {
				continue
			}
			prunedIDs = append(prunedIDs, e.ID)
		}
	}

	return updated, prunedIDs
}

// PromoteOnAccess promotes a memory entry to TierHot when it is accessed.
// If the entry is already TierHot or TierEternal, it is returned unchanged.
// The LastAccess time is always updated.
func PromoteOnAccess(entry MemoryEntry) MemoryEntry {
	entry.LastAccess = time.Now()
	entry.AccessCount++

	if entry.Tier == TierEternal || entry.Tier == TierHot {
		return entry
	}

	entry.Tier = TierHot
	return entry
}

// MarkEternal marks an entry as TierEternal, making it immune to migration
// and pruning. This is a user-initiated action.
func MarkEternal(entry MemoryEntry) MemoryEntry {
	entry.Tier = TierEternal
	return entry
}

// UnmarkEternal removes the TierEternal protection from an entry,
// resetting it to TierHot.
func UnmarkEternal(entry MemoryEntry) MemoryEntry {
	if entry.Tier != TierEternal {
		return entry
	}
	entry.Tier = TierHot
	return entry
}
