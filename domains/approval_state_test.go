package domains

import (
	"testing"
)

func TestApprovalState_Valid(t *testing.T) {
	tests := []struct {
		s  ApprovalState
		ok bool
	}{
		{StateNone, true},
		{StateDrafted, true},
		{StatePendingApproval, true},
		{StateApproved, true},
		{StateModified, true},
		{StateRejected, true},
		{StateCanceled, true},
		{StateExpired, true},
		{"unknown", false},
		{"", false},
	}
	for _, tt := range tests {
		// StateNone (empty string) is valid as the zero-value initial state.
		want := tt.ok
		if tt.s == StateNone {
			want = true
		}
		if got := tt.s.Valid(); got != want {
			t.Errorf("Valid(%q) = %v, want %v", tt.s, got, tt.ok)
		}
	}
}

func TestApprovalState_IsTerminal(t *testing.T) {
	tests := []struct {
		s    ApprovalState
		term bool
	}{
		{StateApproved, true},
		{StateModified, true},
		{StateRejected, true},
		{StateCanceled, true},
		{StateExpired, true},
		{StateDrafted, false},
		{StatePendingApproval, false},
		{StateNone, false},
	}
	for _, tt := range tests {
		if got := tt.s.IsTerminal(); got != tt.term {
			t.Errorf("IsTerminal(%q) = %v, want %v", tt.s, got, tt.term)
		}
	}
}

func TestApprovalState_TransitionAllowed(t *testing.T) {
	tests := []struct {
		from  ApprovalState
		to    ApprovalState
		allow bool
	}{
		// Drafted transitions
		{StateDrafted, StatePendingApproval, true},
		{StateDrafted, StateCanceled, true},
		{StateDrafted, StateApproved, false}, // skip pending
		{StateDrafted, StateRejected, false},

		// Pending approval transitions
		{StatePendingApproval, StateApproved, true},
		{StatePendingApproval, StateModified, true},
		{StatePendingApproval, StateRejected, true},
		{StatePendingApproval, StateExpired, true},
		{StatePendingApproval, StateCanceled, true},
		{StatePendingApproval, StateDrafted, false}, // cannot go back

		// Terminal states
		{StateApproved, StateDrafted, false},
		{StateApproved, StateRejected, false},
		{StateModified, StateApproved, false},
		{StateRejected, StateDrafted, false},
		{StateCanceled, StatePendingApproval, false},
		{StateExpired, StateApproved, false},

		// Same state always allowed
		{StateDrafted, StateDrafted, true},
		{StateApproved, StateApproved, true},
	}
	for _, tt := range tests {
		err := tt.from.TransitionAllowed(tt.to)
		if tt.allow && err != nil {
			t.Errorf("TransitionAllowed(%q→%q) = %v, want nil", tt.from, tt.to, err)
		}
		if !tt.allow && err == nil {
			t.Errorf("TransitionAllowed(%q→%q) = nil, want error", tt.from, tt.to)
		}
	}
}

func TestApprovalRecordState_Approve(t *testing.T) {
	rec := ApprovalRecord{ID: "rec_001"}
	ars := NewApprovalRecordState(rec)

	// Must first submit for approval
	if err := ars.SubmitForApproval(); err != nil {
		t.Fatalf("SubmitForApproval: %v", err)
	}

	// Then approve
	if err := ars.Approve(); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if ars.State != StateApproved {
		t.Errorf("state=%q, want approved", ars.State)
	}
	if ars.Record.Decision != DecisionAdopted {
		t.Errorf("decision=%q, want adopted", ars.Record.Decision)
	}
}

func TestApprovalRecordState_Modify(t *testing.T) {
	rec := ApprovalRecord{ID: "rec_002"}
	ars := NewApprovalRecordState(rec)

	ars.SubmitForApproval()
	if err := ars.Modify("修改后的内容", "建议调整措辞"); err != nil {
		t.Fatalf("Modify: %v", err)
	}
	if ars.State != StateModified {
		t.Errorf("state=%q, want modified", ars.State)
	}
	if ars.Record.ModifiedOutput != "修改后的内容" {
		t.Errorf("modified=%q, want 修改后的内容", ars.Record.ModifiedOutput)
	}
}

func TestApprovalRecordState_Reject(t *testing.T) {
	rec := ApprovalRecord{ID: "rec_003"}
	ars := NewApprovalRecordState(rec)

	ars.SubmitForApproval()
	if err := ars.Reject("缺乏依据"); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if ars.State != StateRejected {
		t.Errorf("state=%q, want rejected", ars.State)
	}
	if ars.Record.Feedback != "缺乏依据" {
		t.Errorf("feedback=%q, want 缺乏依据", ars.Record.Feedback)
	}
}

func TestApprovalRecordState_Cancel(t *testing.T) {
	rec := ApprovalRecord{ID: "rec_004"}
	ars := NewApprovalRecordState(rec)

	if err := ars.Cancel(); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if ars.State != StateCanceled {
		t.Errorf("state=%q, want canceled", ars.State)
	}
}

func TestApprovalRecordState_ApproveWithoutSubmit(t *testing.T) {
	rec := ApprovalRecord{ID: "rec_005"}
	ars := NewApprovalRecordState(rec)

	if err := ars.Approve(); err == nil {
		t.Error("expected error approving without submit")
	}
}
