package a2a

import "fmt"

// ValidateCard checks that an AgentCard has required fields.
func ValidateCard(card AgentCard) error {
	if card.Name == "" {
		return fmt.Errorf("agent card: name is required")
	}
	if card.URL == "" {
		return fmt.Errorf("agent card: url is required")
	}
	for i, skill := range card.Skills {
		if skill.ID == "" {
			return fmt.Errorf("agent card: skill[%d]: id is required", i)
		}
		if skill.Name == "" {
			return fmt.Errorf("agent card: skill[%d]: name is required", i)
		}
		if err := validateSkillParameters(skill.Parameters, i); err != nil {
			return err
		}
	}
	return nil
}

func validateSkillParameters(params map[string]any, skillIndex int) error {
	if params == nil {
		return nil
	}
	schemaType, ok := params["type"]
	if !ok {
		return fmt.Errorf("agent card: skill[%d]: parameters must have a 'type' field (JSON Schema)", skillIndex)
	}
	typeStr, ok := schemaType.(string)
	if !ok {
		return fmt.Errorf("agent card: skill[%d]: parameters 'type' must be a string", skillIndex)
	}
	validTypes := map[string]bool{
		"object":  true,
		"array":   true,
		"string":  true,
		"number":  true,
		"integer": true,
		"boolean": true,
		"null":    true,
	}
	if !validTypes[typeStr] {
		return fmt.Errorf("agent card: skill[%d]: parameters has invalid JSON Schema type %q", skillIndex, typeStr)
	}
	if typeStr == "object" {
		if props, ok := params["properties"]; ok {
			if _, ok := props.(map[string]any); !ok {
				return fmt.Errorf("agent card: skill[%d]: parameters 'properties' must be an object", skillIndex)
			}
		}
	}
	return nil
}
