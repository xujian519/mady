package agentcore

import (
	"testing"
)

func TestCallConfig_Equal(t *testing.T) {
	cases := []struct {
		name string
		a    *CallConfig
		b    *CallConfig
		want bool
	}{
		{"both nil", nil, nil, true},
		{"one nil", &CallConfig{Model: "x"}, nil, false},
		{"same empty", &CallConfig{}, &CallConfig{}, true},
		{"same model", &CallConfig{Model: "m1"}, &CallConfig{Model: "m1"}, true},
		{"different model", &CallConfig{Model: "m1"}, &CallConfig{Model: "m2"}, false},
		{
			"same skills",
			&CallConfig{Skills: []string{"a", "b"}},
			&CallConfig{Skills: []string{"a", "b"}},
			true,
		},
		{
			"different skills len",
			&CallConfig{Skills: []string{"a"}},
			&CallConfig{Skills: []string{"a", "b"}},
			false,
		},
		{
			"different skills content",
			&CallConfig{Skills: []string{"a", "b"}},
			&CallConfig{Skills: []string{"a", "c"}},
			false,
		},
		{
			"same thinking",
			&CallConfig{Thinking: &ThinkingConfig{Effort: ThinkingEffortHigh}},
			&CallConfig{Thinking: &ThinkingConfig{Effort: ThinkingEffortHigh}},
			true,
		},
		{
			"different thinking",
			&CallConfig{Thinking: &ThinkingConfig{Effort: ThinkingEffortHigh}},
			&CallConfig{Thinking: &ThinkingConfig{Effort: ThinkingEffortLow}},
			false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.a.Equal(tc.b)
			if got != tc.want {
				t.Fatalf("Equal = %v, want %v", got, tc.want)
			}
		})
	}
}
