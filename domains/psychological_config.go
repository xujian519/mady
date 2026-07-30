package domains

import "github.com/xujian519/mady/psychological"

// ChatPsychConfig returns a psychological config tuned for chat domain.
// Uses lightweight processing: no distortion detection for B2B professional context.
// Note: SkipDistortionDetection is a TODO placeholder — cognitive distortion
// detection is not yet implemented, so setting it has no behavioral effect.
func ChatPsychConfig() psychological.Config {
	return psychological.Config{SkipDistortionDetection: true}
}

// AssistantPsychConfig returns a psychological config tuned for assistant domain.
// Uses minimal processing for task execution context.
// Note: SkipDistortionDetection is a TODO placeholder — cognitive distortion
// detection is not yet implemented, so setting it has no behavioral effect.
func AssistantPsychConfig() psychological.Config {
	return psychological.Config{SkipDistortionDetection: true}
}

// PatentPsychConfig returns a psychological config tuned for patent domain.
// Note: SkipDistortionDetection is a TODO placeholder — cognitive distortion
// detection is not yet implemented, so setting it has no behavioral effect.
func PatentPsychConfig() psychological.Config {
	return psychological.Config{SkipDistortionDetection: false}
}

// LegalPsychConfig returns a psychological config tuned for legal domain.
// Note: SkipDistortionDetection is a TODO placeholder — cognitive distortion
// detection is not yet implemented, so setting it has no behavioral effect.
func LegalPsychConfig() psychological.Config {
	return psychological.Config{SkipDistortionDetection: false}
}
