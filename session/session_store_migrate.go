package session

import "encoding/json"

// ---------------------------------------------------------------------------
// Version migration
// ---------------------------------------------------------------------------

func migrateEntries(entries []Entry, fromVersion int64) bool {
	changed := false
	if fromVersion < 2 {
		migrateV1ToV2(entries)
		changed = true
	}
	if fromVersion < 3 {
		migrateV2ToV3(entries)
		changed = true
	}
	if fromVersion < 4 {
		migrateV3ToV4(entries)
		changed = true
	}
	return changed
}

func migrateV1ToV2(entries []Entry) {
	var lastID string
	for i := range entries {
		if entries[i].ID == "" {
			entries[i].ID = generateID()
		}
		if entries[i].ParentID == "" && lastID != "" {
			entries[i].ParentID = lastID
		}
		lastID = entries[i].ID
		entries[i].Version = 2
	}
}

func migrateV2ToV3(entries []Entry) {
	for i := range entries {
		if entries[i].Type == EntryCompaction {
			var raw map[string]any
			if json.Unmarshal(entries[i].Data, &raw) == nil {
				if idxVal, ok := raw["firstKeptEntryIndex"]; ok {
					delete(raw, "firstKeptEntryIndex")

					// Compute first_kept_entry_id from the entries list using
					// the removed firstKeptEntryIndex as an index into the list.
					if idxFloat, ok := idxVal.(float64); ok {
						idx := int(idxFloat)
						if entriesList, ok := raw["entries"].([]any); ok && idx >= 0 && idx < len(entriesList) {
							if idStr, ok := entriesList[idx].(string); ok {
								raw["first_kept_entry_id"] = idStr
							}
						}
					}

					//nolint:errchkjson // raw is map[string]any (dynamic migration data)
					updated, _ := json.Marshal(raw)
					entries[i].Data = updated
				}
			}
		}
		entries[i].Version = 3
	}
}

func migrateV3ToV4(entries []Entry) {
	for i := range entries {
		if entries[i].Type == EntryMessage {
			var msg map[string]any
			if json.Unmarshal(entries[i].Data, &msg) == nil {
				if role, ok := msg["role"].(string); ok && role == "hookMessage" {
					msg["role"] = "custom"
					//nolint:errchkjson // msg is map[string]any (dynamic migration data)
					updated, _ := json.Marshal(msg)
					entries[i].Data = updated
				}
			}
		}
		entries[i].Version = 4
	}
}
