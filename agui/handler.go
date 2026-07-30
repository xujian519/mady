package agui

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xujian519/mady/agentcore"
)

// Handler serves AGUI protocol requests over HTTP/SSE.
type Handler struct {
	mu     sync.RWMutex
	config agentcore.Config
}

// NewHandler creates a new AGUI Handler with the given base configuration.
func NewHandler(cfg agentcore.Config) *Handler {
	return &Handler{config: cfg}
}

// UpdateConfig atomically replaces the handler's agent configuration.
func (h *Handler) UpdateConfig(cfg agentcore.Config) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.config = cfg
}

func (h *Handler) snapshotConfig() agentcore.Config {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.config
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.handleRun(w, r)
	case http.MethodGet:
		h.handleCapabilities(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *Handler) handleCapabilities(w http.ResponseWriter, _ *http.Request) {
	cfg := h.snapshotConfig()
	caps := CapabilitiesFromConfig(cfg)
	writeJSON(w, http.StatusOK, caps)
}

//nolint:gocognit
func (h *Handler) handleRun(w http.ResponseWriter, r *http.Request) {
	var input RunAgentInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
		return
	}

	cfg := h.snapshotConfig()
	threadID := input.ThreadID
	if threadID == "" {
		threadID = generateID("thread")
	}
	runID := input.RunID
	if runID == "" {
		runID = generateID("run")
	}

	callCfg := callConfigFromInput(input)
	agent, err := agentcore.LoadAgent(r.Context(), cfg, agentcore.LoadAgentOptions{
		ThreadID:          threadID,
		CallCfg:           callCfg,
		ThreadCfgProvider: threadCfgProviderFromConfig(cfg),
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer agent.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	if threadID != "" && cfg.Store != nil {
		hasState, err := cfg.Store.Has(r.Context(), threadID)
		if err == nil && hasState {
			snapAgent := agentcore.New(cfg)
			if err := snapAgent.LoadState(r.Context(), threadID); err == nil {
				msgs := snapAgent.State().Messages()
				snapshot := MessagesSnapshotEvent{
					BaseEvent: baseEvent(EventMessagesSnapshot, time.Now()),
					Messages:  MessagesFromAgent(msgs),
				}
				writeSSE(w, flusher, string(EventMessagesSnapshot), snapshot)
				snapAgent.Close()
			}
		}
	}

	converter := NewConverterWithParent(threadID, runID, input.ParentRunID)

	if input.State != nil {
		writeSSE(w, flusher, string(EventStateSnapshot), converter.StateSnapshot(time.Now(), input.State))
	}

	if len(input.Resume) > 0 {
		for _, r := range input.Resume {
			agent.Steer(agentcore.Message{
				Role:    agentcore.RoleSystem,
				Content: fmt.Sprintf("Interrupt %s resolved with status: %s", r.InterruptID, r.Status),
			})
		}
	}

	// Register the SSE writer as a scoped handler. The agent may be pooled
	// and reused across requests; without unregister, this closure would
	// leak onto the agent and keep writing to this dead ResponseWriter on
	// subsequent requests.
	var mu sync.Mutex
	unregister := agent.OnAll(func(e agentcore.Event) {
		agEvents := converter.Convert(e)
		mu.Lock()
		defer mu.Unlock()
		for _, agEv := range agEvents {
			evtType := extractEventType(agEv)
			writeSSE(w, flusher, evtType, agEv)
		}
	})
	defer unregister()

	// 在每个 turn 结束后发射状态快照，使前端能追踪 Agent 状态变化。
	// Snapshot() 内部持有 RLock，线程安全。
	unregisterSnap := agent.On(agentcore.EventTurnEnd, func(_ agentcore.Event) {
		snap := agent.State().Snapshot()
		mu.Lock()
		writeSSE(w, flusher, string(EventStateSnapshot),
			converter.StateSnapshot(time.Now(), snap))
		mu.Unlock()
	})
	defer unregisterSnap()

	var message string
	if len(input.Messages) > 0 {
		last := input.Messages[len(input.Messages)-1]
		if last.Role == MessageRoleUser {
			message = last.Content
		}
	}
	if message == "" {
		writeSSE(w, flusher, string(EventRunError), RunErrorEvent{
			BaseEvent: baseEvent(EventRunError, time.Now()),
			ThreadID:  threadID,
			RunID:     runID,
			Message:   "no user message provided",
		})
		return
	}

	if cfg.Provider == nil {
		writeSSE(w, flusher, string(EventRunError), RunErrorEvent{
			BaseEvent: baseEvent(EventRunError, time.Now()),
			ThreadID:  threadID,
			RunID:     runID,
			Message:   "no provider configured",
		})
		return
	}

	_, runErr := agent.Run(r.Context(), message)
	if threadID != "" && cfg.Store != nil {
		if err := agent.SaveState(r.Context(), threadID); err != nil {
			slog.Warn("agui: save state failed", "err", err)
		}
	}

	if runErr != nil {
		for _, agEv := range converter.closeAll(time.Now()) {
			evtType := extractEventType(agEv)
			writeSSE(w, flusher, evtType, agEv)
		}
		writeSSE(w, flusher, string(EventRunFinished), converter.RunFinished(time.Now()))
	}
}

// callConfigFromInput extracts per-call configuration from the AGUI input.
// Currently only checks whether Tools or State were provided (which would
// indicate a per-call override is expected), but CallConfig does not yet
// support injecting per-call tools — that requires a future extension to
// CallConfig or a separate agent API. input.State is already handled
// directly by handleRun via SSE StateSnapshot events.
func callConfigFromInput(input RunAgentInput) *agentcore.CallConfig {
	if input.Tools == nil && input.State == nil {
		return nil
	}
	return &agentcore.CallConfig{}
}

func threadCfgProviderFromConfig(cfg agentcore.Config) agentcore.ThreadConfigProvider {
	if cfg.Store == nil {
		return nil
	}
	p, ok := cfg.Store.(agentcore.ThreadConfigProvider)
	if !ok {
		return nil
	}
	return p
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, eventType string, data any) {
	payload, marshalErr := json.Marshal(data)
	if marshalErr != nil {
		slog.Warn("agui: writeSSE marshal failed", "err", marshalErr)
		_, _ = fmt.Fprintf(w, "event: %s\ndata: {}\n\n", eventType)
		flusher.Flush()
		return
	}
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, payload)
	flusher.Flush()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("agui: writeJSON failed", "err", err)
	}
}

func extractEventType(ev any) string {
	type typed interface {
		GetType() EventType
	}
	if t, ok := ev.(typed); ok {
		return string(t.GetType())
	}
	return string(EventCustom)
}

var idCounter atomic.Uint64

func generateID(prefix string) string {
	n := idCounter.Add(1)
	return fmt.Sprintf("%s_%d", prefix, n)
}
