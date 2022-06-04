package wasm_exec

import (
	"context"
	"time"
)

// stateKey is a context.Context Value key. The value must be a state pointer.
type stateKey struct{}

func getState(ctx context.Context) *state {
	return ctx.Value(stateKey{}).(*state)
}

// state holds state used by the "go" imports used by wasm_exec.
// Note: This is module-scoped.
type state struct {
	nextCallbackTimeoutID uint32
	scheduledTimeouts     map[uint32]*time.Timer
	values                *values
	_pendingEvent         *event
}

func (s *state) clear() {
	s.nextCallbackTimeoutID = 0
	for k := range s.scheduledTimeouts {
		delete(s.scheduledTimeouts, k)
	}
	s.values.values = s.values.values[:0]
	s.values.goRefCounts = s.values.goRefCounts[:0]
	for k := range s.values.ids {
		delete(s.values.ids, k)
	}
	s.values.idPool = s.values.idPool[:0]
	s._pendingEvent = nil
}

// scheduleEvent schedules an event onto another goroutine after d duration and
// returns a handle to remove it (removeEvent).
func (s *state) scheduleEvent(d time.Duration, f func()) uint32 {
	id := s.nextCallbackTimeoutID
	s.nextCallbackTimeoutID++
	// TODO: this breaks the sandbox (proc.checkTimers is shared), so should
	// be substitutable with a different impl.
	s.scheduledTimeouts[id] = time.AfterFunc(d, f)
	return id
}

// removeEvent removes an event previously scheduled with scheduleEvent or
// returns nil, if it was already removed.
func (s *state) removeEvent(id uint32) *time.Timer {
	t, ok := s.scheduledTimeouts[id]
	if ok {
		delete(s.scheduledTimeouts, id)
		return t
	}
	return nil
}

func (s *state) clearTimeoutEvent(id uint32) {
	if t := s.removeEvent(id); t != nil {
		if !t.Stop() {
			<-t.C
		}
	}
}
