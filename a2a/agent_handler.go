package a2a

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"slices"

	"github.com/xujian519/mady/agentcore"
)

// ---------------------------------------------------------------------------
// DefaultAgentHandler bridges agentcore.Agent to A2A protocol.
// ---------------------------------------------------------------------------

type DefaultAgentHandler struct {
	card        AgentCard
	agent       *agentcore.Agent
	config      agentcore.Config
	taskTimeout time.Duration

	tasksMu  sync.RWMutex
	tasks    map[string]*Task
	maxTasks int

	pushMu  sync.RWMutex
	pushCfg map[string]*PushNotificationConfig

	notifier      *PushNotifier
	publisher     TaskUpdatePublisher
	inputRequired func(output string) bool

	execSem     chan struct{}
	cancelMu    sync.Mutex
	cancelFuncs map[string]context.CancelFunc
}

// defaultMaxTasks caps how many task records DefaultAgentHandler retains in
// memory. Once exceeded, the oldest terminal-state tasks (completed, failed,
// or canceled) are evicted first; in-flight tasks are never evicted.
const defaultMaxTasks = 10000

func NewDefaultAgentHandler(card AgentCard, agent *agentcore.Agent, cfg agentcore.Config) *DefaultAgentHandler {
	return &DefaultAgentHandler{
		card:        card,
		agent:       agent,
		config:      cfg,
		taskTimeout: defaultTaskTimeout,
		tasks:       make(map[string]*Task),
		maxTasks:    defaultMaxTasks,
		pushCfg:     make(map[string]*PushNotificationConfig),
		notifier:    NewPushNotifier(),
		execSem:     make(chan struct{}, 10),
		cancelFuncs: make(map[string]context.CancelFunc),
	}
}

// SetMaxTasks overrides how many task records are retained in memory before
// the oldest terminal-state tasks start being evicted. Values <= 0 reset it
// to the default (10000).
func (h *DefaultAgentHandler) SetMaxTasks(n int) {
	h.tasksMu.Lock()
	defer h.tasksMu.Unlock()
	if n <= 0 {
		n = defaultMaxTasks
	}
	h.maxTasks = n
	h.evictOldTasksLocked()
}

// evictOldTasksLocked drops the oldest terminal-state tasks (and their
// associated push-notification config) once len(h.tasks) exceeds
// h.maxTasks. Callers must already hold h.tasksMu for writing. Tasks that
// are not yet in a terminal state are never evicted, so clients polling or
// continuing them never lose access.
func (h *DefaultAgentHandler) evictOldTasksLocked() {
	limit := h.maxTasks
	if limit <= 0 {
		limit = defaultMaxTasks
	}
	for len(h.tasks) > limit {
		oldestID := ""
		var oldestTime time.Time
		for id, t := range h.tasks {
			if !isTerminalState(t.State) {
				continue
			}
			var ts time.Time
			if n := len(t.History); n > 0 {
				ts = t.History[n-1].Timestamp
			}
			if oldestID == "" || ts.Before(oldestTime) {
				oldestID = id
				oldestTime = ts
			}
		}
		if oldestID == "" {
			// Nothing evictable: every remaining task is still in flight.
			return
		}
		delete(h.tasks, oldestID)
		h.pushMu.Lock()
		delete(h.pushCfg, oldestID)
		h.pushMu.Unlock()
	}
}

func (h *DefaultAgentHandler) SetMaxConcurrency(n int) {
	// Not safe for concurrent use with runAgent — call before any SendTask.
	if n <= 0 {
		n = 10
	}
	h.execSem = make(chan struct{}, n)
}

func (h *DefaultAgentHandler) SetTaskTimeout(d time.Duration) {
	h.taskTimeout = d
}

func (h *DefaultAgentHandler) SetUpdatePublisher(p TaskUpdatePublisher) {
	h.publisher = p
}

func (h *DefaultAgentHandler) SetInputRequiredPredicate(fn func(output string) bool) {
	h.inputRequired = fn
}

func (h *DefaultAgentHandler) Card() AgentCard { return h.card }

func (h *DefaultAgentHandler) SendTask(ctx context.Context, req SendTaskRequest) (*Task, error) {
	h.tasksMu.RLock()
	existingTask, exists := h.tasks[req.ID]
	h.tasksMu.RUnlock()

	if exists {
		if isTerminalState(existingTask.State) {
			return nil, fmt.Errorf("task %q is already in terminal state %q", req.ID, existingTask.State)
		}
		if existingTask.State != TaskStateInputRequired && existingTask.State != TaskStateSubmitted {
			return nil, fmt.Errorf("task %q is in state %q, cannot append message", req.ID, existingTask.State)
		}
		return h.continueTask(ctx, existingTask, req)
	}

	return h.newTask(ctx, req)
}

func (h *DefaultAgentHandler) newTask(ctx context.Context, req SendTaskRequest) (*Task, error) {
	input := req.inputOverride
	if input == "" {
		for _, p := range req.Message.Parts {
			if p.Type == PartTypeText {
				input += p.Text
			}
		}
	}

	task := &Task{
		ID:        req.ID,
		SessionID: req.SessionID,
		State:     TaskStateSubmitted,
		Messages:  []Message{req.Message},
		Metadata:  req.Metadata,
		History: []TaskStatus{
			{State: TaskStateSubmitted, Timestamp: time.Now()},
		},
	}

	h.tasksMu.Lock()
	h.tasks[task.ID] = task
	h.tasksMu.Unlock()

	return h.runAgent(ctx, task, input)
}

func (h *DefaultAgentHandler) continueTask(ctx context.Context, task *Task, req SendTaskRequest) (*Task, error) {
	input := req.inputOverride
	if input == "" {
		for _, p := range req.Message.Parts {
			if p.Type == PartTypeText {
				input += p.Text
			}
		}
	}

	h.tasksMu.Lock()
	defer h.tasksMu.Unlock()

	// Re-validate state under write lock — CancelTask may have acted since SendTask's RLock.
	if isTerminalState(task.State) || (task.State != TaskStateInputRequired && task.State != TaskStateSubmitted) {
		return nil, fmt.Errorf("task %q state changed to %q, cannot continue", req.ID, task.State)
	}

	task.Messages = append(task.Messages, req.Message)
	task.State = TaskStateWorking
	task.History = append(task.History, TaskStatus{
		State:     TaskStateWorking,
		Timestamp: time.Now(),
	})

	h.publish(task.ID, &TaskUpdateEvent{Result: &Task{ID: task.ID, State: TaskStateWorking}, Final: false})

	return h.runAgent(ctx, task, input)
}

func (h *DefaultAgentHandler) runAgent(ctx context.Context, task *Task, input string) (*Task, error) {
	select {
	case h.execSem <- struct{}{}:
		defer func() { <-h.execSem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	h.tasksMu.Lock()
	if task.State != TaskStateWorking {
		task.State = TaskStateWorking
		task.History = append(task.History, TaskStatus{State: TaskStateWorking, Timestamp: time.Now()})
		h.tasksMu.Unlock()
		h.publish(task.ID, &TaskUpdateEvent{Result: &Task{ID: task.ID, State: TaskStateWorking}, Final: false})
	} else {
		h.tasksMu.Unlock()
	}

	var unsub func()
	if h.publisher != nil {
		unsub = h.subscribeAgentEvents(task.ID)
	}

	runCtx, cancel := context.WithTimeout(ctx, h.taskTimeout)
	defer cancel()

	h.cancelMu.Lock()
	h.cancelFuncs[task.ID] = cancel
	h.cancelMu.Unlock()

	defer func() {
		h.cancelMu.Lock()
		delete(h.cancelFuncs, task.ID)
		h.cancelMu.Unlock()
	}()

	output, err := h.agent.Run(runCtx, input)

	if unsub != nil {
		unsub()
	}

	h.tasksMu.Lock()

	needNotify := true
	switch {
	case err != nil:
		// Preserve TaskStateCanceled if CancelTask already acted.
		if task.State != TaskStateCanceled {
			task.State = TaskStateFailed
			task.History = append(task.History, TaskStatus{
				State:     TaskStateFailed,
				Timestamp: time.Now(),
			})
		}
	case h.inputRequired != nil && h.inputRequired(output):
		task.State = TaskStateInputRequired
		task.Messages = append(task.Messages, Message{
			Role:  string(RoleAgent),
			Parts: []Part{NewTextPart(output)},
		})
		task.History = append(task.History, TaskStatus{
			State:     TaskStateInputRequired,
			Timestamp: time.Now(),
		})
	default:
		task.State = TaskStateCompleted
		task.Messages = append(task.Messages, Message{
			Role:  string(RoleAgent),
			Parts: []Part{NewTextPart(output)},
		})
		task.Artifacts = append(task.Artifacts, Artifact{
			Name:  "output",
			Parts: []Part{NewTextPart(output)},
		})
		task.History = append(task.History, TaskStatus{
			State:     TaskStateCompleted,
			Timestamp: time.Now(),
		})
	}

	h.evictOldTasksLocked()
	h.tasksMu.Unlock()

	if needNotify {
		h.notify(task)
	}
	return task, nil
}

func (h *DefaultAgentHandler) publish(taskID string, ev *TaskUpdateEvent) {
	if h.publisher != nil {
		h.publisher.PublishTaskUpdate(taskID, ev)
	}
}

func (h *DefaultAgentHandler) subscribeAgentEvents(taskID string) (unsub func()) {
	if h.publisher == nil {
		return func() {}
	}

	var active int32 = 1
	handler := func(e agentcore.Event) {
		if atomic.LoadInt32(&active) == 0 {
			return
		}
		switch ev := e.(type) {
		case *agentcore.MessageDeltaEvent:
			h.publish(taskID, &TaskUpdateEvent{
				Result: &Task{
					ID:    taskID,
					State: TaskStateWorking,
				},
				Artifact: &Artifact{
					Parts:     []Part{NewTextPart(ev.Delta)},
					Append:    boolPtr(true),
					LastChunk: boolPtr(false),
				},
				Final: false,
			})
		case *agentcore.ToolCallStartEvent:
			h.publish(taskID, &TaskUpdateEvent{
				Result: &Task{
					ID:    taskID,
					State: TaskStateWorking,
				},
				Final: false,
			})
		}
	}

	// Register the handler and return a cleanup that BOTH flips the active
	// flag (so any in-flight dispatch sees the handler is done) AND unregisters
	// it from the agent's event bus. Without the unregister, a pooled/reused
	// agent would accumulate stale handlers across tasks — they'd keep firing
	// into h.publish for tasks that have already finished.
	unregister := h.agent.OnAll(handler)
	return func() {
		atomic.StoreInt32(&active, 0)
		unregister()
	}
}

func boolPtr(b bool) *bool { return &b }

func (h *DefaultAgentHandler) notify(task *Task) {
	h.pushMu.RLock()
	cfg, ok := h.pushCfg[task.ID]
	h.pushMu.RUnlock()
	if !ok || cfg == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := h.notifier.Notify(ctx, cfg, task); err != nil {
		slog.Default().Warn("push notification failed", "task_id", task.ID, "error", err)
	}
}

// GetTask implements AgentHandler.
func (h *DefaultAgentHandler) GetTask(ctx context.Context, req GetTaskRequest) (*Task, error) {
	h.tasksMu.RLock()
	defer h.tasksMu.RUnlock()

	task, ok := h.tasks[req.ID]
	if !ok {
		return nil, fmt.Errorf("task %q not found", req.ID)
	}

	t := *task
	if req.HistoryLength > 0 && len(t.History) > req.HistoryLength {
		offset := len(t.History) - req.HistoryLength
		t.History = t.History[offset:]
	}
	return &t, nil
}

// CancelTask implements AgentHandler.
func (h *DefaultAgentHandler) CancelTask(ctx context.Context, req CancelTaskRequest) (*Task, error) {
	h.tasksMu.Lock()

	task, ok := h.tasks[req.ID]
	if !ok {
		h.tasksMu.Unlock()
		return nil, fmt.Errorf("task %q not found", req.ID)
	}

	if isTerminalState(task.State) {
		h.tasksMu.Unlock()
		return nil, fmt.Errorf("task %q is already in terminal state %q", req.ID, task.State)
	}

	h.cancelMu.Lock()
	if cf, ok := h.cancelFuncs[req.ID]; ok {
		cf()
		delete(h.cancelFuncs, req.ID)
	}
	h.cancelMu.Unlock()

	task.State = TaskStateCanceled
	task.History = append(task.History, TaskStatus{
		State:     TaskStateCanceled,
		Timestamp: time.Now(),
	})

	h.evictOldTasksLocked()
	h.tasksMu.Unlock()

	// notify outside tasksMu to avoid blocking task ops on HTTP push latency.
	h.notify(task)
	return task, nil
}

// QueryTasks implements AgentHandler.
func (h *DefaultAgentHandler) QueryTasks(ctx context.Context, req QueryTasksRequest) (*QueryTasksResult, error) {
	h.tasksMu.RLock()
	defer h.tasksMu.RUnlock()

	var result []*Task
	for _, task := range h.tasks {
		if req.SessionID != "" && task.SessionID != req.SessionID {
			continue
		}
		if req.State != "" && task.State != req.State {
			continue
		}
		t := *task
		result = append(result, &t)
	}

	slices.SortFunc(result, func(a, b *Task) int {
		var ta, tb time.Time
		if len(a.History) > 0 {
			ta = a.History[0].Timestamp
		}
		if len(b.History) > 0 {
			tb = b.History[0].Timestamp
		}
		return ta.Compare(tb)
	})

	if req.Limit > 0 && len(result) > req.Limit {
		result = result[:req.Limit]
	}

	if result == nil {
		result = []*Task{}
	}

	return &QueryTasksResult{Tasks: result}, nil
}

// SetPushNotification implements AgentHandler.
func (h *DefaultAgentHandler) SetPushNotification(ctx context.Context, req SetPushNotificationRequest) error {
	h.pushMu.Lock()
	defer h.pushMu.Unlock()
	h.pushCfg[req.ID] = &req.Config
	return nil
}

// GetPushNotification implements AgentHandler.
func (h *DefaultAgentHandler) GetPushNotification(ctx context.Context, taskID string) (*PushNotificationConfig, error) {
	h.pushMu.RLock()
	defer h.pushMu.RUnlock()
	cfg, ok := h.pushCfg[taskID]
	if !ok {
		return nil, fmt.Errorf("no push notification config for task %q", taskID)
	}
	return cfg, nil
}
