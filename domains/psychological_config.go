package domains

import "github.com/xujian519/mady/psychological"

// ChatPsychConfig returns a psychological config tuned for chat domain.
//
// Chat uses lightweight psychological processing:
//   - VAD/OCC pipeline runs for emotion-aware tone adjustment
//   - Distortion detection is skipped (no "cognitive distortion" labels
//     exposed to users — inappropriate for B2B professional tools)
//   - No LLM verification (EnableLLM=false) to avoid extra API calls
func ChatPsychConfig() psychological.Config {
	return psychological.Config{
		SessionID:               "chat",
		SkipDistortionDetection: true,
		EnableLLM:               false,
	}
}

// AssistantPsychConfig returns a psychological config tuned for assistant domain.
//
// Assistant uses minimal psychological processing:
//   - SDT tracking for motivation/engagement monitoring
//   - Distortion detection skipped (task execution doesn't need CBT analysis)
//   - VAD/OCC runs at default thresholds for tone adaptation
func AssistantPsychConfig() psychological.Config {
	return psychological.Config{
		SessionID:               "assistant",
		SkipDistortionDetection: true,
		EnableLLM:               false,
	}
}

// PatentPsychConfig returns a psychological config tuned for patent domain.
//
// Patent domain needs full psychological pipeline for professional client
// interaction monitoring, but with careful output filtering.
func PatentPsychConfig() psychological.Config {
	return psychological.Config{
		SessionID:               "patent",
		SkipDistortionDetection: false,
		EnableLLM:               false,
	}
}

// LegalPsychConfig returns a psychological config tuned for legal domain.
func LegalPsychConfig() psychological.Config {
	return psychological.Config{
		SessionID:               "legal",
		SkipDistortionDetection: false,
		EnableLLM:               false,
	}
}
