package caseindex

import (
	"context"
	"fmt"
	"time"
)

// --- 事件管理 ---

// AddEvent 记录一条案件事件。
func (ci *CaseIndex) AddEvent(ctx context.Context, caseID, eventType, eventData string) error {
	ci.mu.Lock()
	defer ci.mu.Unlock()

	_, err := ci.db.ExecContext(ctx, `
		INSERT INTO case_events (case_id, event_type, event_data) VALUES (?, ?, ?)
	`, caseID, eventType, eventData)
	if err != nil {
		return fmt.Errorf("case_index: add event: %w", err)
	}
	return nil
}

// GetEvents 返回案件的事件历史。
func (ci *CaseIndex) GetEvents(ctx context.Context, caseID string) ([]CaseEvent, error) {
	rows, err := ci.db.QueryContext(ctx, `
		SELECT case_id, event_type, event_data, event_date
		FROM case_events WHERE case_id = ? ORDER BY event_date
	`, caseID)
	if err != nil {
		return nil, fmt.Errorf("case_index: get events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var events []CaseEvent
	for rows.Next() {
		var e CaseEvent
		var dateStr string
		if err := rows.Scan(&e.CaseID, &e.EventType, &e.EventData, &dateStr); err != nil {
			return nil, err
		}
		e.EventDate, _ = time.Parse(time.RFC3339, dateStr)
		events = append(events, e)
	}
	return events, rows.Err()
}
