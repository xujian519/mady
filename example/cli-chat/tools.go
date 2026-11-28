package main

import (
	"context"
	"encoding/json"

	"github.com/xujian519/mady/agentcore"
)

func weatherTool() *agentcore.Tool {
	return &agentcore.Tool{
		Name:        "get_weather",
		Description: "Get the current weather for a given city",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"location": map[string]any{
					"type":        "string",
					"description": "City name, e.g. Tokyo",
				},
			},
			"required":             []string{"location"},
			"additionalProperties": false,
		},
		Func: func(_ context.Context, args json.RawMessage) (any, error) {
			var p struct {
				Location string `json:"location"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return nil, err
			}
			return map[string]any{
				"location":    p.Location,
				"temperature": "22°C",
				"condition":   "sunny",
				"humidity":    "45%",
			}, nil
		},
	}
}

func calculatorTool() *agentcore.Tool {
	return &agentcore.Tool{
		Name:        "calculator",
		Description: "Evaluate a simple math expression",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"expression": map[string]any{
					"type":        "string",
					"description": "Math expression, e.g. 7*8",
				},
			},
			"required":             []string{"expression"},
			"additionalProperties": false,
		},
		Func: func(_ context.Context, args json.RawMessage) (any, error) {
			var p struct {
				Expression string `json:"expression"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return nil, err
			}
			return map[string]string{
				"expression": p.Expression,
				"result":     "56",
				"note":       "stub calculator",
			}, nil
		},
	}
}
