package chat

// state.go defines an explicit finite-state machine over ChatApp interaction
// states, decoupled from the imperative event handlers in chat_app_*.go.
//
// Why: the handlers in chat_app_stream.go / chat_app_tool.go mutate chatModel
// fields (Running, StreamID, ActiveTools) directly, so the "what state are we
// in" question is scattered across branches. This module makes the state
// lattice explicit and pure — Transition(s, event) -> (newState, []sideEffect)
// has no side effects, so it can be table-tested and used as a regression
// oracle for the handler behavior.
//
// The states deliberately mirror what the user sees:
//
//   StateIdle           — nothing running, editor is the focus
//   StateStreaming      — assistant text is streaming (StreamID != "")
//   StateToolRunning    — one or more tool calls are in flight
//   StateAwaitingConfirm— an approval gate is blocking (review mode)
//   StateCompacting     — context compaction is running
//
// This is a progressive refactor: today the handlers still own the mutable
// model; this FSM is the documented spec + test target. A later pass can have
// the handlers delegate to Transition and apply the returned side effects.

// AppState is one interaction state of the ChatApp FSM.
type AppState int

const (
	StateIdle AppState = iota
	StateStreaming
	StateToolRunning
	StateAwaitingConfirm
	StateCompacting
)

// String returns a human-readable state name for diagnostics and tests.
func (s AppState) String() string {
	switch s {
	case StateStreaming:
		return "streaming"
	case StateToolRunning:
		return "tool-running"
	case StateAwaitingConfirm:
		return "awaiting-confirm"
	case StateCompacting:
		return "compacting"
	default:
		return "idle"
	}
}

// eventKind is a stable, FSM-local enumeration of the ChatEvent types that
// drive state transitions. Using a local enum (rather than the ChatEvent
// interface values) keeps Transition pure and free of type assertions — the
// caller maps its ChatEvent to an eventKind before calling Transition.
type eventKind int

const (
	evtAgentStart eventKind = iota
	evtMessageDelta
	evtToolStart
	evtToolEnd
	evtTurnStart
	evtTurnEnd
	evtHandoffStart
	evtHandoffEnd
	evtCompactionStart
	evtCompactionEnd
	evtAgentEnd
	evtAgentError
	evtAutoRetry
	evtApprovalRequest
	evtApprovalDecision
	// evtUnknown is the safe default for unrecognized events: Transition has
	// no case for it, so it returns the state unchanged (a true no-op). This
	// must NOT be evtAgentStart, which would spuriously flip Idle→Streaming.
	evtUnknown
)

// Transition returns the AppState that results from applying one event in the
// given state. It is a pure function: same (state, event) always yields the
// same result, with no I/O or mutation.
//
// The lattice (events that change state):
//
//	Idle            --AgentStart-->        Streaming
//	Streaming       --MessageDelta-->      Streaming      (steady)
//	Streaming       --ToolStart-->         ToolRunning
//	ToolRunning     --ToolEnd-->           Streaming      (back to text or idle)
//	Streaming       --CompactionStart-->   Compacting
//	Compacting      --CompactionEnd-->     Streaming
//	*               --ApprovalRequest-->   AwaitingConfirm
//	AwaitingConfirm --ApprovalDecision-->  Streaming (or Idle if no stream)
//	Streaming       --AgentEnd/Error-->    Idle
//	Streaming       --TurnEnd-->           Streaming (turn boundary, not end)
//
// Events that don't change the current state are no-ops (return s unchanged).
func Transition(s AppState, e eventKind) AppState {
	switch s {
	case StateIdle:
		if e == evtAgentStart {
			return StateStreaming
		}
		if e == evtApprovalRequest {
			return StateAwaitingConfirm
		}
		return s

	case StateStreaming:
		switch e {
		case evtToolStart:
			return StateToolRunning
		case evtCompactionStart:
			return StateCompacting
		case evtApprovalRequest:
			return StateAwaitingConfirm
		case evtAgentEnd, evtAgentError:
			return StateIdle
		case evtMessageDelta, evtTurnEnd, evtHandoffStart, evtHandoffEnd:
			return StateStreaming
		}
		return s

	case StateToolRunning:
		switch e {
		case evtToolEnd:
			return StateStreaming
		case evtCompactionStart:
			return StateCompacting
		case evtApprovalRequest:
			return StateAwaitingConfirm
		case evtAgentEnd, evtAgentError:
			return StateIdle
		}
		return s

	case StateCompacting:
		if e == evtCompactionEnd {
			return StateStreaming
		}
		if e == evtAgentEnd || e == evtAgentError {
			return StateIdle
		}
		return s

	case StateAwaitingConfirm:
		if e == evtApprovalDecision {
			return StateStreaming
		}
		if e == evtAgentEnd || e == evtAgentError {
			return StateIdle
		}
		return s
	}
	return s
}

// EventKindFor maps a ChatEvent to its FSM eventKind. This is the bridge
// between the imperative ChatEvent interface and the pure FSM: handlers can
// call Transition(state, EventKindFor(e)) to decide the next state without
// type-switching themselves.
func EventKindFor(e ChatEvent) eventKind {
	switch e.(type) {
	case AgentStartChatEvent:
		return evtAgentStart
	case MessageDeltaChatEvent:
		return evtMessageDelta
	case ToolCallStartChatEvent:
		return evtToolStart
	case ToolCallEndChatEvent:
		return evtToolEnd
	case TurnStartChatEvent:
		return evtTurnStart
	case TurnEndChatEvent:
		return evtTurnEnd
	case HandoffStartChatEvent:
		return evtHandoffStart
	case HandoffEndChatEvent:
		return evtHandoffEnd
	case CompactionStartChatEvent:
		return evtCompactionStart
	case CompactionEndChatEvent:
		return evtCompactionEnd
	case AgentEndChatEvent:
		return evtAgentEnd
	case AgentErrorChatEvent:
		return evtAgentError
	case AutoRetryChatEvent:
		return evtAutoRetry
	}
	// Unknown events map to evtUnknown, for which Transition has no case —
	// it returns the current state unchanged (a genuine no-op). Returning
	// evtAgentStart here would wrongly flip Idle→Streaming on any future
	// event type added without an explicit mapping.
	return evtUnknown
}
