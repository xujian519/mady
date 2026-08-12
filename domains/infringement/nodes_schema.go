package infringement

// --- JSON Schemas ---

func claimScopeSchema() map[string]any {
	return map[string]any{
		jsFieldType: jsTypeObject,
		jsFieldProperties: map[string]any{
			"interpreted_scope":      map[string]any{jsFieldType: jsTypeString},
			"key_terms":              map[string]any{jsFieldType: jsTypeArray, jsFieldItems: map[string]any{jsFieldType: jsTypeObject, jsFieldProperties: map[string]any{"term": map[string]any{jsFieldType: jsTypeString}, "interpretation": map[string]any{jsFieldType: jsTypeString}, "evidence_source": map[string]any{jsFieldType: jsTypeString, jsFieldEnum: []string{"intrinsic", "extrinsic"}}}}},
			"disclaimers_identified": map[string]any{jsFieldType: jsTypeArray, jsFieldItems: map[string]any{jsFieldType: jsTypeString}},
		},
		jsFieldRequired: []string{"interpreted_scope", "key_terms"},
	}
}

func featureDecompSchema() map[string]any {
	return map[string]any{
		jsFieldType: jsTypeObject,
		jsFieldProperties: map[string]any{
			"claim_features":   map[string]any{jsFieldType: jsTypeArray, jsFieldItems: map[string]any{jsFieldType: jsTypeString}},
			"product_features": map[string]any{jsFieldType: jsTypeArray, jsFieldItems: map[string]any{jsFieldType: jsTypeString}},
		},
		jsFieldRequired: []string{"claim_features", "product_features"},
	}
}

func literalSchema() map[string]any {
	return map[string]any{
		jsFieldType: jsTypeObject,
		jsFieldProperties: map[string]any{
			"all_elements_met": map[string]any{jsFieldType: jsTypeBoolean},
			"feature_mapping":  map[string]any{jsFieldType: jsTypeArray, jsFieldItems: map[string]any{jsFieldType: jsTypeObject, jsFieldProperties: map[string]any{"claim_feature": map[string]any{jsFieldType: jsTypeString}, "product_feature": map[string]any{jsFieldType: jsTypeString}, "match_type": map[string]any{jsFieldType: jsTypeString, jsFieldEnum: []string{jsFieldLiteral, "equivalent", "missing"}}, "match_reasoning": map[string]any{jsFieldType: jsTypeString}}}},
			"missing_features": map[string]any{jsFieldType: jsTypeArray, jsFieldItems: map[string]any{jsFieldType: jsTypeString}},
			"extra_features":   map[string]any{jsFieldType: jsTypeArray, jsFieldItems: map[string]any{jsFieldType: jsTypeString}},
		},
		jsFieldRequired: []string{"all_elements_met", "feature_mapping"},
	}
}

func equivalenceSchema() map[string]any {
	return map[string]any{
		jsFieldType: jsTypeObject,
		jsFieldProperties: map[string]any{
			"equivalent_features": map[string]any{jsFieldType: jsTypeArray, jsFieldItems: map[string]any{jsFieldType: jsTypeObject, jsFieldProperties: map[string]any{"claim_feature": map[string]any{jsFieldType: jsTypeString}, "product_feature": map[string]any{jsFieldType: jsTypeString}, "same_means": map[string]any{jsFieldType: jsTypeBoolean}, "same_function": map[string]any{jsFieldType: jsTypeBoolean}, "same_effect": map[string]any{jsFieldType: jsTypeBoolean}, "non_obviousness": map[string]any{jsFieldType: jsTypeBoolean}, "is_equivalent": map[string]any{jsFieldType: jsTypeBoolean}, "reasoning": map[string]any{jsFieldType: jsTypeString}}}},
			"estoppel_applied":    map[string]any{jsFieldType: jsTypeBoolean},
			"estoppel_details":    map[string]any{jsFieldType: jsTypeString},
			"dedication_applied":  map[string]any{jsFieldType: jsTypeBoolean},
			"dedication_details":  map[string]any{jsFieldType: jsTypeString},
		},
		jsFieldRequired: []string{"equivalent_features", "estoppel_applied"},
	}
}

func verdictSchema() map[string]any {
	return map[string]any{
		jsFieldType: jsTypeObject,
		jsFieldProperties: map[string]any{
			"conclusion":   map[string]any{jsFieldType: jsTypeString, jsFieldEnum: []string{"infringed", "not_infringed", "uncertain"}},
			"likelihood":   map[string]any{jsFieldType: jsTypeNumber, "minimum": 0, "maximum": 1},
			"basis":        map[string]any{jsFieldType: jsTypeArray, jsFieldItems: map[string]any{jsFieldType: jsTypeString, jsFieldEnum: []string{jsFieldLiteral, jsFieldEquivalence}}},
			"key_findings": map[string]any{jsFieldType: jsTypeArray, jsFieldItems: map[string]any{jsFieldType: jsTypeString}},
			"risk_level":   map[string]any{jsFieldType: jsTypeString, jsFieldEnum: []string{jsValHigh, jsValMedium, jsValLow}},
		},
		jsFieldRequired: []string{"conclusion", "likelihood", "basis", "key_findings", "risk_level"},
	}
}

func defenseSchema() map[string]any {
	return map[string]any{
		jsFieldType: jsTypeObject,
		jsFieldProperties: map[string]any{
			"defenses": map[string]any{jsFieldType: jsTypeArray, jsFieldItems: map[string]any{jsFieldType: jsTypeObject, jsFieldProperties: map[string]any{"defense_type": map[string]any{jsFieldType: jsTypeString}, "applicable": map[string]any{jsFieldType: jsTypeBoolean}, "viability_rating": map[string]any{jsFieldType: jsTypeString, jsFieldEnum: []string{jsValHigh, jsValMedium, jsValLow, "none"}}, "analysis": map[string]any{jsFieldType: jsTypeString}, "evidence_needed": map[string]any{jsFieldType: jsTypeArray, jsFieldItems: map[string]any{jsFieldType: jsTypeString}}, "legal_basis": map[string]any{jsFieldType: jsTypeString}}}},
		},
		jsFieldRequired: []string{"defenses"},
	}
}

func remedySchema() map[string]any {
	return map[string]any{
		jsFieldType: jsTypeObject,
		jsFieldProperties: map[string]any{
			"damage_estimate":     damageEstimateSchema(),
			"injunction_analysis": injunctionAnalysisSchema(),
			"punitive_risk":       punitiveRiskSchema(),
		},
		jsFieldRequired: []string{"damage_estimate", "injunction_analysis"},
	}
}

func damageEstimateSchema() map[string]any {
	return map[string]any{
		jsFieldType: jsTypeObject,
		jsFieldProperties: map[string]any{
			"method":            map[string]any{jsFieldType: jsTypeString},
			"estimated_amount":  map[string]any{jsFieldType: jsTypeNumber},
			"range_low":         map[string]any{jsFieldType: jsTypeNumber},
			"range_high":        map[string]any{jsFieldType: jsTypeNumber},
			"calculation_basis": map[string]any{jsFieldType: jsTypeString},
		},
	}
}

func injunctionAnalysisSchema() map[string]any {
	injunctionFactorsSchema := map[string]any{
		jsFieldType: jsTypeObject,
		jsFieldProperties: map[string]any{
			"likelihood_of_success": map[string]any{jsFieldType: jsTypeString},
			"irreparable_harm":      map[string]any{jsFieldType: jsTypeString},
			"balance_of_hardships":  map[string]any{jsFieldType: jsTypeString},
			"public_interest":       map[string]any{jsFieldType: jsTypeString},
			"bond_required":         map[string]any{jsFieldType: jsTypeNumber},
		},
	}
	return map[string]any{
		jsFieldType: jsTypeObject,
		jsFieldProperties: map[string]any{
			"preliminary_injunction": injunctionFactorsSchema,
			"permanent_injunction":   injunctionFactorsSchema,
		},
	}
}

func punitiveRiskSchema() map[string]any {
	return map[string]any{
		jsFieldType: jsTypeObject,
		jsFieldProperties: map[string]any{
			"willfulness":     map[string]any{jsFieldType: jsTypeString},
			"multiplier_low":  map[string]any{jsFieldType: jsTypeNumber},
			"multiplier_high": map[string]any{jsFieldType: jsTypeNumber},
			"analysis":        map[string]any{jsFieldType: jsTypeString},
		},
	}
}

func strategySchema() map[string]any {
	return map[string]any{
		jsFieldType: jsTypeObject,
		jsFieldProperties: map[string]any{
			"recommended_actions":   map[string]any{jsFieldType: jsTypeArray, jsFieldItems: map[string]any{jsFieldType: jsTypeObject, jsFieldProperties: map[string]any{"action": map[string]any{jsFieldType: jsTypeString}, "priority": map[string]any{jsFieldType: jsTypeString, jsFieldEnum: []string{"immediate", "short_term", "long_term"}}, "rationale": map[string]any{jsFieldType: jsTypeString}, "risk_level": map[string]any{jsFieldType: jsTypeString}}}},
			"jurisdiction_analysis": map[string]any{jsFieldType: jsTypeObject, jsFieldProperties: map[string]any{"recommended_venues": map[string]any{jsFieldType: jsTypeArray, jsFieldItems: map[string]any{jsFieldType: jsTypeString}}, "rationale": map[string]any{jsFieldType: jsTypeString}}},
			jsFieldTimeline:         map[string]any{jsFieldType: jsTypeArray, jsFieldItems: map[string]any{jsFieldType: jsTypeObject, jsFieldProperties: map[string]any{"event": map[string]any{jsFieldType: jsTypeString}, "timeframe": map[string]any{jsFieldType: jsTypeString}, "criticality": map[string]any{jsFieldType: jsTypeString}}}},
			"settlement_assessment": map[string]any{jsFieldType: jsTypeObject, jsFieldProperties: map[string]any{"recommendation": map[string]any{jsFieldType: jsTypeString}, "range_low": map[string]any{jsFieldType: jsTypeNumber}, "range_high": map[string]any{jsFieldType: jsTypeNumber}, "key_factors": map[string]any{jsFieldType: jsTypeArray, jsFieldItems: map[string]any{jsFieldType: jsTypeString}}}},
			"invalidation_route":    map[string]any{jsFieldType: jsTypeObject, jsFieldProperties: map[string]any{"grounds": map[string]any{jsFieldType: jsTypeArray, jsFieldItems: map[string]any{jsFieldType: jsTypeString}}, "prior_art_refs": map[string]any{jsFieldType: jsTypeArray, jsFieldItems: map[string]any{jsFieldType: jsTypeString}}, "success_chance": map[string]any{jsFieldType: jsTypeString}, jsFieldTimeline: map[string]any{jsFieldType: jsTypeString}}},
		},
		jsFieldRequired: []string{"recommended_actions", jsFieldTimeline},
	}
}
