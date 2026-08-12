package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/xujian519/mady/pkg/util"
)

// --- JSON serialization for events with error fields ---

// MarshalJSON serializes the error event, converting the error to a string.
func (e AgentErrorEvent) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type      EventType `json:"type"`
		Timestamp time.Time `json:"timestamp"`
		Error     string    `json:"error"`
		ErrorType string    `json:"error_type,omitempty"`
	}{e.Kind, e.At, util.ErrorString(e.Err), errorType(e.Err)})
}

// UnmarshalJSON deserializes the error event, reconstructing error from type/string.
func (e *AgentErrorEvent) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type      EventType `json:"type"`
		Timestamp time.Time `json:"timestamp"`
		Error     string    `json:"error"`
		ErrorType string    `json:"error_type,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.Kind = raw.Type
	e.At = raw.Timestamp
	if raw.Error != "" {
		e.Err = reconstructError(raw.Error, raw.ErrorType)
	}
	return nil
}

// MarshalJSON serializes the tool call end event.
func (e ToolCallEndEvent) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type       EventType     `json:"type"`
		Timestamp  time.Time     `json:"timestamp"`
		ToolCallID string        `json:"tool_call_id"`
		ToolName   string        `json:"tool_name"`
		Result     string        `json:"result"`
		Duration   time.Duration `json:"duration"`
		Error      string        `json:"error,omitempty"`
		ErrorType  string        `json:"error_type,omitempty"`
	}{e.Kind, e.At, e.ToolCallID, e.ToolName, e.Result, e.Duration, util.ErrorString(e.Err), errorType(e.Err)})
}

// UnmarshalJSON deserializes the tool call end event.
func (e *ToolCallEndEvent) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type       EventType     `json:"type"`
		Timestamp  time.Time     `json:"timestamp"`
		ToolCallID string        `json:"tool_call_id"`
		ToolName   string        `json:"tool_name"`
		Result     string        `json:"result"`
		Duration   time.Duration `json:"duration"`
		Error      string        `json:"error,omitempty"`
		ErrorType  string        `json:"error_type,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.Kind = raw.Type
	e.At = raw.Timestamp
	e.ToolCallID = raw.ToolCallID
	e.ToolName = raw.ToolName
	e.Result = raw.Result
	e.Duration = raw.Duration
	if raw.Error != "" {
		e.Err = reconstructError(raw.Error, raw.ErrorType)
	}
	return nil
}

// MarshalJSON serializes the handoff end event.
func (e HandoffEndEvent) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type        EventType     `json:"type"`
		Timestamp   time.Time     `json:"timestamp"`
		TargetAgent string        `json:"target_agent"`
		Output      string        `json:"output"`
		Duration    time.Duration `json:"duration"`
		Invisible   bool          `json:"invisible"`
		Error       string        `json:"error,omitempty"`
		ErrorType   string        `json:"error_type,omitempty"`
	}{e.Kind, e.At, e.TargetAgent, e.Output, e.Duration, e.Invisible, util.ErrorString(e.Err), errorType(e.Err)})
}

// UnmarshalJSON deserializes the handoff end event.
func (e *HandoffEndEvent) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type        EventType     `json:"type"`
		Timestamp   time.Time     `json:"timestamp"`
		TargetAgent string        `json:"target_agent"`
		Output      string        `json:"output"`
		Duration    time.Duration `json:"duration"`
		Invisible   bool          `json:"invisible"`
		Error       string        `json:"error,omitempty"`
		ErrorType   string        `json:"error_type,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.Kind = raw.Type
	e.At = raw.Timestamp
	e.TargetAgent = raw.TargetAgent
	e.Output = raw.Output
	e.Duration = raw.Duration
	e.Invisible = raw.Invisible
	if raw.Error != "" {
		e.Err = reconstructError(raw.Error, raw.ErrorType)
	}
	return nil
}

// MarshalJSON serializes the auto-retry event.
func (e AutoRetryEvent) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type       EventType     `json:"type"`
		Timestamp  time.Time     `json:"timestamp"`
		Attempt    int64         `json:"attempt"`
		MaxRetries int64         `json:"max_retries"`
		Delay      time.Duration `json:"delay"`
		Error      string        `json:"error"`
		ErrorType  string        `json:"error_type,omitempty"`
	}{e.Kind, e.At, e.Attempt, e.MaxRetries, e.Delay, util.ErrorString(e.Err), errorType(e.Err)})
}

// UnmarshalJSON deserializes the auto-retry event.
func (e *AutoRetryEvent) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type       EventType     `json:"type"`
		Timestamp  time.Time     `json:"timestamp"`
		Attempt    int64         `json:"attempt"`
		MaxRetries int64         `json:"max_retries"`
		Delay      time.Duration `json:"delay"`
		Error      string        `json:"error"`
		ErrorType  string        `json:"error_type,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.Kind = raw.Type
	e.At = raw.Timestamp
	e.Attempt = raw.Attempt
	e.MaxRetries = raw.MaxRetries
	e.Delay = raw.Delay
	if raw.Error != "" {
		e.Err = reconstructError(raw.Error, raw.ErrorType)
	}
	return nil
}

// errorType returns a stable type identifier for an error.
func errorType(err error) string {
	if err == nil {
		return ""
	}
	switch err {
	case context.Canceled:
		return "context.Canceled"
	case context.DeadlineExceeded:
		return "context.DeadlineExceeded"
	}
	return ""
}

// reconstructError rebuilds an error from its string representation and type hint.
func reconstructError(msg, errType string) error {
	switch errType {
	case "context.Canceled":
		return context.Canceled
	case "context.DeadlineExceeded":
		return context.DeadlineExceeded
	}
	return errors.New(msg)
}
