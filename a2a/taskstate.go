package a2a

import (
	"log/slog"
	"runtime/debug"
	"sort"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
type taskSubState struct {
	mu      sync.RWMutex
	subs    []chan *TaskUpdateEvent
	history []historyEntry
	nextSeq int
}

type historyEntry struct {
	event *TaskUpdateEvent
	seq   int
}

func (s *Server) getTaskState(taskID string) *taskSubState {
	s.taskStatesMu.RLock()
	ts, ok := s.taskStates[taskID]
	s.taskStatesMu.RUnlock()
	if ok {
		return ts
	}

	s.taskStatesMu.Lock()
	defer s.taskStatesMu.Unlock()

	if ts, ok = s.taskStates[taskID]; ok {
		return ts
	}

	ts = &taskSubState{}
	s.taskStates[taskID] = ts
	return ts
}

func (s *Server) startCleanup() {
	s.cleanupTicker = time.NewTicker(s.taskTTL / 2)
	s.cleanupStop = make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Default().Error("[a2a] cleanup goroutine panicked", "panic", r, "stack", string(debug.Stack()))
			}
		}()
		for {
			select {
			case <-s.cleanupTicker.C:
				s.purgeOldTasks()
			case <-s.cleanupStop:
				return
			}
		}
	}()
}

func (s *Server) purgeOldTasks() {
	cutoff := time.Now().Add(-s.taskTTL)

	s.taskStatesMu.Lock()
	var toDelete []string
	for id, ts := range s.taskStates {
		ts.mu.RLock()
		hist := ts.history
		if len(hist) == 0 {
			ts.mu.RUnlock()
			continue
		}
		lastEv := hist[len(hist)-1]
		ts.mu.RUnlock()

		var lastUpdate time.Time
		if lastEv.event.Result != nil && len(lastEv.event.Result.History) > 0 {
			lastUpdate = lastEv.event.Result.History[len(lastEv.event.Result.History)-1].Timestamp
		}
		if lastUpdate.IsZero() {
			continue
		}
		if lastUpdate.Before(cutoff) {
			toDelete = append(toDelete, id)
		}
	}

	for _, id := range toDelete {
		ts := s.taskStates[id]
		ts.mu.Lock()
		for _, ch := range ts.subs {
			close(ch)
		}
		ts.subs = nil
		removed := len(ts.history)
		ts.history = nil
		ts.mu.Unlock()
		s.totalHistSize.Add(-int64(removed))
		delete(s.taskStates, id)
		s.logger.Debug("purged old task history", "task_id", id)
	}
	s.taskStatesMu.Unlock()

	if s.sessionMgr != nil && s.sessionTTL > 0 {
		sessionCutoff := time.Now().Add(-s.sessionTTL)
		count := s.sessionMgr.PurgeStale(sessionCutoff)
		if count > 0 {
			s.logger.Debug("purged stale sessions", "count", count)
		}
	}
}

func isTerminalState(state TaskState) bool {
	switch state {
	case TaskStateCompleted, TaskStateFailed, TaskStateCanceled:
		return true
	default:
		return false
	}
}

// recordTask stores a task snapshot in event history for resubscription.
// Lock ordering: ts.mu → (release) → taskStatesMu → t.mu(RLock).
func (s *Server) recordTask(task *Task) {
	ts := s.getTaskState(task.ID)
	ts.mu.Lock()
	ev := &TaskUpdateEvent{Result: task, Final: isTerminalState(task.State)}
	ts.nextSeq++
	trimmed := 0
	if len(ts.history) >= s.maxHistoryLen {
		trimmed = len(ts.history) - s.maxHistoryLen + 1
		ts.history = ts.history[trimmed:]
	}
	ts.history = append(ts.history, historyEntry{event: ev, seq: ts.nextSeq})
	ts.mu.Unlock()

	newSize := s.totalHistSize.Add(1 - int64(trimmed))

	if newSize > int64(s.maxTotalHist) {
		s.taskStatesMu.Lock()
		// Batch collect: one pass to get all task ages, O(n), then evict
		// the oldest tasks in sorted order instead of re-scanning per eviction.
		type taskAge struct {
			id        string
			firstTS   time.Time
			totalSize int
		}
		ages := make([]taskAge, 0, len(s.taskStates))
		for id, t := range s.taskStates {
			t.mu.RLock()
			if len(t.history) > 0 {
				first := t.history[0].event
				if first.Result != nil && len(first.Result.History) > 0 {
					ages = append(ages, taskAge{
						id:        id,
						firstTS:   first.Result.History[0].Timestamp,
						totalSize: len(t.history),
					})
				}
			}
			t.mu.RUnlock()
		}
		sort.Slice(ages, func(i, j int) bool {
			return ages[i].firstTS.Before(ages[j].firstTS)
		})
		for _, a := range ages {
			if s.totalHistSize.Load() <= int64(s.maxTotalHist) {
				break
			}
			ots := s.taskStates[a.id]
			if ots == nil {
				continue
			}
			ots.mu.Lock()
			removed := len(ots.history)
			for _, ch := range ots.subs {
				close(ch)
			}
			ots.subs = nil
			ots.history = nil
			ots.mu.Unlock()
			s.totalHistSize.Add(-int64(removed))
			delete(s.taskStates, a.id)
		}
		s.taskStatesMu.Unlock()
	}
}

func (s *Server) PublishTaskUpdate(taskID string, ev *TaskUpdateEvent) {
	ts := s.getTaskState(taskID)
	ts.mu.Lock()
	ts.nextSeq++
	trimmed := 0
	if len(ts.history) >= s.maxHistoryLen {
		trimmed = len(ts.history) - s.maxHistoryLen + 1
		ts.history = ts.history[trimmed:]
	}
	ts.history = append(ts.history, historyEntry{event: ev, seq: ts.nextSeq})
	chans := make([]chan *TaskUpdateEvent, len(ts.subs))
	copy(chans, ts.subs)
	ts.mu.Unlock()

	s.totalHistSize.Add(1 - int64(trimmed))

	for _, ch := range chans {
		select {
		case ch <- ev:
		default:
			slog.Default().Warn("a2a: subscriber channel full, event dropped", "task_id", taskID)
		}
	}
}
