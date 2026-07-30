// Package psychological provides a lightweight VAD (Valence-Arousal-Dominance)
// emotion space engine for analyzing user sentiment and adapting agent dialog
// strategies accordingly in the Mady agent runtime.
//
// # Current Capabilities
//
//   - Keyword-based text signal extraction: sentiment polarity, uncertainty,
//     blame direction, perceived control, surprise level, goal importance.
//   - VAD emotion vector calculation via direct signal→VAD mapping.
//   - Emotion classification into 6 categories (excited, hopeful, satisfied,
//     anxious, frustrated, disappointed, neutral).
//   - Dialog strategy matching (empathetic, professional, encouraging, neutral,
//     calming) based on VAD coordinates.
//   - Extension integration (agentcore.Extension) for automatic psychological
//     context injection into system prompts via TransformContext.
//   - An analyze_emotion tool for on-demand emotion analysis.
//
// # Limitations
//
//   - The VAD model is a simplified keyword-based approximation, not a full
//     continuous emotion space model. Precision depends on keyword coverage.
//   - Cognitive distortion detection is NOT implemented. The
//     SkipDistortionDetection configuration field is a placeholder reserved for
//     future use; setting it to true/false currently produces no behavioral
//     difference.
//   - This package is suitable for Chat Agent lightweight emotion sensing.
//     It is NOT suitable for psychological diagnosis, clinical mental health
//     assessment, or any scenario requiring regulated medical advice.
//
// # Usage
//
//	result := psychological.ExecuteFullPipeline(userText, &PipelineConfig{})
//	contextBlock := psychological.BuildContextBlock(result)
//
// The Extension can be registered with the agent runtime for automatic injection:
//
//	ext := psychological.NewExtension(cfg)
//	agent.RegisterExtension(ext)
package psychological
