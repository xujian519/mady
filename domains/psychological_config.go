package domains

import "github.com/xujian519/mady/psychological"

// ChatPsychConfig returns a psychological config tuned for chat domain.
// Uses lightweight processing: no distortion detection for B2B professional context.
func ChatPsychConfig() psychological.Config {
	return psychological.Config{SkipDistortionDetection: true}
}

// AssistantPsychConfig returns a psychological config tuned for assistant domain.
// Uses minimal processing for task execution context.
func AssistantPsychConfig() psychological.Config {
	return psychological.Config{SkipDistortionDetection: true}
}

// PatentPsychConfig returns a psychological config tuned for patent domain.
func PatentPsychConfig() psychological.Config {
	return psychological.Config{SkipDistortionDetection: false}
}

// LegalPsychConfig returns a psychological config tuned for legal domain.
func LegalPsychConfig() psychological.Config {
	return psychological.Config{SkipDistortionDetection: false}
}
