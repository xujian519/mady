package evaluate

import "testing"

func TestJudgeConsistency_Heuristic(t *testing.T) {
	jc := JudgeConsistency{}

	tests := []struct {
		name       string
		prediction string
		reference  string
		want       float64
	}{
		{
			name:       "high overlap - agree",
			prediction: "权利要求1不具备新颖性，因为对比文件1公开了相同的技术方案",
			reference:  "权利要求1不具备新颖性。对比文件1公开了相同的技术方案",
			want:       1.0,
		},
		{
			name:       "low overlap - disagree",
			prediction: "这个发明非常有创意值得授权",
			reference:  "权利要求1不具备新颖性因为已被对比文件公开",
			want:       0.0,
		},
		{
			name:       "empty reference - agree",
			prediction: "some answer",
			reference:  "",
			want:       1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jc.Compute(tt.prediction, tt.reference)
			if got != tt.want {
				t.Fatalf("Compute() = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestJudgeConsistency_CustomJudge(t *testing.T) {
	jc := JudgeConsistency{
		Judge: func(prediction, reference string) bool {
			return prediction == reference
		},
	}

	if jc.Compute("a", "a") != 1.0 {
		t.Fatal("expected 1.0 for identical strings")
	}
	if jc.Compute("a", "b") != 0.0 {
		t.Fatal("expected 0.0 for different strings")
	}
}

func TestJudgeConsistency_Name(t *testing.T) {
	jc := JudgeConsistency{}
	if jc.Name() != "judge_consistency" {
		t.Fatal("unexpected name")
	}
}

func TestGuardrailFalseNegativeRate(t *testing.T) {
	g := GuardrailFalseNegativeRate{TotalHighRisk: 10, FlaggedHighRisk: 8}

	if g.Rate() != 0.2 {
		t.Fatalf("Rate() = %f, want 0.2", g.Rate())
	}
	if g.Score() != 0.8 {
		t.Fatalf("Score() = %f, want 0.8", g.Score())
	}

	gZero := GuardrailFalseNegativeRate{}
	if gZero.Rate() != 0 {
		t.Fatal("expected 0 rate for zero total")
	}
}

func TestAdoptionRate(t *testing.T) {
	a := AdoptionRate{Adopted: 6, Modified: 3, Rejected: 1}

	if a.Total() != 10 {
		t.Fatalf("Total() = %d, want 10", a.Total())
	}
	if a.FullyAdopted() != 0.6 {
		t.Fatalf("FullyAdopted() = %f, want 0.6", a.FullyAdopted())
	}
	if a.Accepted() != 0.9 {
		t.Fatalf("Accepted() = %f, want 0.9", a.Accepted())
	}
	if a.RejectedRate() != 0.1 {
		t.Fatalf("RejectedRate() = %f, want 0.1", a.RejectedRate())
	}

	aZero := AdoptionRate{}
	if aZero.FullyAdopted() != 0 {
		t.Fatal("expected 0 for empty adoption rate")
	}
}
