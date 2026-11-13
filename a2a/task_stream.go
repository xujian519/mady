package a2a

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// TaskStream
// ---------------------------------------------------------------------------

// TaskStream receives SSE updates for a task with automatic reconnection.
type TaskStream struct {
	ch       chan *TaskUpdateEvent
	body     io.ReadCloser
	cancel   context.Context
	err      error
	mu       sync.Mutex
	lastID   string
	client   *Client
	taskID   string
	maxRetry int
	retryNum int
}

// Recv returns the next task update event. Returns nil, false when done.
func (s *TaskStream) Recv() (*TaskUpdateEvent, bool) {
	ev, ok := <-s.ch
	return ev, ok
}

// Err returns any error that occurred during streaming.
func (s *TaskStream) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Close closes the stream.
func (s *TaskStream) Close() error {
	return s.body.Close()
}

func (s *TaskStream) readLoop() {
	defer close(s.ch)
	defer func() { _ = s.body.Close() }()

	for {
		decoder := NewSSEDecoder(s.body)
		for {
			select {
			case <-s.cancel.Done():
				return
			default:
			}

			ev, err := decoder.Next()
			if err != nil {
				if s.tryReconnect() {
					continue
				}
				s.mu.Lock()
				s.err = err
				s.mu.Unlock()
				return
			}
			if ev == nil {
				return
			}

			if ev.ID != "" {
				s.mu.Lock()
				s.lastID = ev.ID
				s.mu.Unlock()
			}

			var update TaskUpdateEvent
			if err := json.Unmarshal([]byte(ev.Data), &update); err != nil {
				// 事件解码失败→记入流错误并结束读循环，由 Next() 的调用方上抛。
				s.mu.Lock()
				s.err = fmt.Errorf("decode sse event: %w", err)
				s.mu.Unlock()
				return
			}

			select {
			case s.ch <- &update:
				if update.Final {
					return
				}
			case <-s.cancel.Done():
				return
			}
		}
	}
}

// tryReconnect 重连一次流；返回 false 表示放弃重连（重试次数耗尽、
// 取消、请求构造或响应非 200），此时由 readLoop 记录 s.err 结束流。
func (s *TaskStream) tryReconnect() bool {
	if s.client == nil || s.taskID == "" {
		return false
	}

	s.mu.Lock()
	lastID := s.lastID
	retries := s.maxRetry
	s.mu.Unlock()

	if retries <= 0 {
		return false
	}

	s.mu.Lock()
	s.maxRetry = retries - 1
	s.retryNum++
	attempt := s.retryNum
	s.mu.Unlock()

	_ = s.body.Close()

	backoff := time.Duration(500<<min(attempt-1, 5)) * time.Millisecond
	if backoff > 30*time.Second {
		backoff = 30 * time.Second
	}
	timer := time.NewTimer(backoff)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-s.cancel.Done():
		return false
	}

	rpcReq := JSONRPCRequest{JSONRPC: "2.0", ID: s.client.nextID(), Method: "tasks/resubscribe"}
	params, marshalErr := json.Marshal(map[string]string{"id": s.taskID})
	if marshalErr != nil {
		// 参数序列化失败→返回 false 由调用方按「订阅失败」上报。
		return false
	}
	rpcReq.Params = params

	body, err := json.Marshal(rpcReq)
	if err != nil {
		// 请求序列化失败→返回 false，由调用方按「订阅失败」处理。
		return false
	}

	httpReq, err := http.NewRequestWithContext(s.cancel, http.MethodPost, s.client.baseURL+"/", bytes.NewReader(body))
	if err != nil {
		// 请求构造失败→返回 false，由调用方按「订阅失败」处理。
		return false
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if lastID != "" {
		httpReq.Header.Set("Last-Event-ID", lastID)
	}
	s.client.setAuthHeaders(httpReq)

	httpResp, err := s.client.httpClient.Do(httpReq)
	if err != nil {
		// 传输失败→返回 false，由调用方按「订阅失败」处理（不在此处重试）。
		return false
	}

	if httpResp.StatusCode != http.StatusOK {
		_ = httpResp.Body.Close()
		return false
	}

	s.body = httpResp.Body
	return true
}

// ---------------------------------------------------------------------------
// SSE Decoder
// ---------------------------------------------------------------------------

// SSEEvent represents a single Server-Sent Event.
type SSEEvent struct {
	ID    string
	Event string
	Data  string
}

// SSEDecoder decodes a stream of Server-Sent Events.
type SSEDecoder struct {
	r *bufio.Reader
}

// NewSSEDecoder creates an SSE decoder from a reader.
func NewSSEDecoder(r io.Reader) *SSEDecoder {
	return &SSEDecoder{r: bufio.NewReader(r)}
}

// Next reads the next SSE event from the stream.
func (d *SSEDecoder) Next() (*SSEEvent, error) {
	var ev SSEEvent
	var dataLines []string

	for {
		line, err := d.r.ReadString('\n')
		if err != nil {
			// 已读到数据行却遇到 EOF→按「最后一个事件无结尾空行」补齐返回，不算错误。
			if err == io.EOF && len(dataLines) > 0 {
				ev.Data = strings.Join(dataLines, "\n")
				return &ev, nil
			}
			return nil, err
		}

		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")

		if line == "" {
			if len(dataLines) > 0 {
				ev.Data = strings.Join(dataLines, "\n")
				return &ev, nil
			}
			continue
		}

		if strings.HasPrefix(line, "id:") {
			ev.ID = strings.TrimPrefix(line, "id:")
			ev.ID = strings.TrimSpace(ev.ID)
			continue
		}
		if strings.HasPrefix(line, "event:") {
			ev.Event = strings.TrimPrefix(line, "event:")
			ev.Event = strings.TrimSpace(ev.Event)
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			if len(data) > 0 && data[0] == ' ' {
				data = data[1:]
			}
			dataLines = append(dataLines, data)
			continue
		}
	}
}
