package lib

import (
	"encoding/json"
	"testing"
)

// rtkGroupingPluginConfig builds a minimal config.json containing a single rtk
// plugin entry with the given config block, exercising the allOf/if/then plugin
// schema branch for name=rtk.
func rtkGroupingPluginConfig(configBlock string) string {
	return `{
		"plugins": [{
			"name": "rtk",
			"enabled": true,
			"config": ` + configBlock + `
		}]
	}`
}

// TestRTKPluginConfig_Schema_GroupingFields pins the phase-2 grouping config
// contract (V-transports-1): enable_grouping (bool), grouping_threshold
// (int, minimum 2) and apply_to_assistant_messages (bool) must be accepted by
// the schema; grouping_threshold below 2 must be rejected; and the
// additionalProperties: false semantics must be preserved.
//
// TDD red phase: the first sub-test (valid config with new fields) will fail
// because the config.schema.json rtk block does not yet declare these three
// fields — unknown keys are rejected by additionalProperties: false. This is
// the expected red signal.
func TestRTKPluginConfig_Schema_GroupingFields(t *testing.T) {
	// 1. A config carrying all three phase-2 fields must pass schema validation.
	// TDD red phase: this assertion FAILS today because config.schema.json does
	// not yet declare enable_grouping / grouping_threshold /
	// apply_to_assistant_messages, so additionalProperties: false rejects them.
	// The dev phase must add the three fields to the rtk config block, after
	// which this test turns green.
	validConfig := rtkGroupingPluginConfig(`{
		"enabled": true,
		"intensity": "standard",
		"apply_to_tool_results": true,
		"apply_to_code_blocks": false,
		"max_lines_per_result": 120,
		"max_chars_per_result": 12000,
		"dedup_threshold": 3,
		"preserve_cache_control": true,
		"enable_grouping": true,
		"grouping_threshold": 3,
		"apply_to_assistant_messages": false
	}`)
	if err := ValidateConfigSchema([]byte(validConfig), loadLocalSchema(t)); err != nil {
		t.Errorf("valid rtk config with grouping fields should pass schema validation, got: %v", err)
	}

	// 2. grouping_threshold below the schema minimum (2) must be rejected.
	// Red phase: currently rejected because the field is unknown (additionalProperties: false).
	// Dev phase: rejected by minimum: 2. Both satisfy the assertion.
	invalidThresholdConfig := rtkGroupingPluginConfig(`{
		"enabled": true,
		"intensity": "standard",
		"enable_grouping": true,
		"grouping_threshold": 1
	}`)
	if err := ValidateConfigSchema([]byte(invalidThresholdConfig), loadLocalSchema(t)); err == nil {
		t.Error("rtk config with grouping_threshold: 1 should be rejected (schema minimum: 2)")
	}

	// 3. Undeclared fields must still be rejected by additionalProperties: false.
	unknownKeyConfig := rtkGroupingPluginConfig(`{
		"enabled": true,
		"intensity": "standard",
		"enable_grouping": true,
		"grouping_threshold": 3,
		"grouping_unknown_field": "must be rejected"
	}`)
	if err := ValidateConfigSchema([]byte(unknownKeyConfig), loadLocalSchema(t)); err == nil {
		t.Error("unknown key in rtk config should be rejected by schema additionalProperties: false")
	}

	// 4. Empty config block (only the implicit defaults) must still pass.
	emptyConfig := rtkGroupingPluginConfig(`{}`)
	if err := ValidateConfigSchema([]byte(emptyConfig), loadLocalSchema(t)); err != nil {
		t.Errorf("empty rtk config block should pass validation, got: %v", err)
	}
}

// TestRTKConfigSchema_GroupingFieldDefinitions verifies the three phase-2 fields
// are actually declared in the rtk config block of config.schema.json with the
// required types and constraints (V-transports-1).
//
// TDD red phase: this test will fail because the fields do not yet exist in the
// schema — this is the expected red signal.
func TestRTKConfigSchema_GroupingFieldDefinitions(t *testing.T) {
	schemaBytes := loadLocalSchema(t)
	var schemaObj map[string]interface{}
	if err := json.Unmarshal(schemaBytes, &schemaObj); err != nil {
		t.Fatalf("failed to parse schema JSON: %v", err)
	}

	// Navigate to plugins.items → allOf → rtk if/then branch → config.properties
	plugins, ok := schemaObj["properties"].(map[string]interface{})["plugins"].(map[string]interface{})
	if !ok {
		t.Fatal("schema missing plugins property")
	}
	items, ok := plugins["items"].(map[string]interface{})
	if !ok {
		t.Fatal("schema plugins missing items")
	}
	allOf, ok := items["allOf"].([]interface{})
	if !ok {
		t.Fatal("schema plugins.items missing allOf")
	}

	// Find the rtk branch in allOf
	var rtkThen map[string]interface{}
	for _, branch := range allOf {
		b, ok := branch.(map[string]interface{})
		if !ok {
			continue
		}
		ifBlock, ok := b["if"].(map[string]interface{})
		if !ok {
			continue
		}
		props, ok := ifBlock["properties"].(map[string]interface{})
		if !ok {
			continue
		}
		nameProp, ok := props["name"].(map[string]interface{})
		if !ok {
			continue
		}
		if nameProp["const"] == "rtk" {
			rtkThen, ok = b["then"].(map[string]interface{})
			break
		}
	}
	if rtkThen == nil {
		t.Fatal("schema missing rtk if/then branch")
	}

	configProps, ok := rtkThen["properties"].(map[string]interface{})["config"].(map[string]interface{})["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("rtk then branch missing config.properties")
	}

	// Check enable_grouping: type boolean
	egProp, exists := configProps["enable_grouping"]
	if !exists {
		t.Error("rtk config schema missing field 'enable_grouping' — schema field name must match Config struct JSON tag")
	} else {
		egObj, ok := egProp.(map[string]interface{})
		if !ok {
			t.Error("enable_grouping schema definition is not an object")
		} else if egObj["type"] != "boolean" {
			t.Errorf("enable_grouping schema type = %v, want 'boolean'", egObj["type"])
		}
	}

	// Check grouping_threshold: type integer, minimum 2
	gtProp, exists := configProps["grouping_threshold"]
	if !exists {
		t.Error("rtk config schema missing field 'grouping_threshold' — schema field name must match Config struct JSON tag")
	} else {
		gtObj, ok := gtProp.(map[string]interface{})
		if !ok {
			t.Error("grouping_threshold schema definition is not an object")
		} else {
			if gtObj["type"] != "integer" {
				t.Errorf("grouping_threshold schema type = %v, want 'integer'", gtObj["type"])
			}
			if min, hasMin := gtObj["minimum"]; !hasMin {
				t.Error("grouping_threshold schema missing 'minimum' — must be >= 2")
			} else if minVal, ok := min.(float64); !ok || minVal != 2 {
				t.Errorf("grouping_threshold schema minimum = %v, want 2", min)
			}
		}
	}

	// Check apply_to_assistant_messages: type boolean
	aamProp, exists := configProps["apply_to_assistant_messages"]
	if !exists {
		t.Error("rtk config schema missing field 'apply_to_assistant_messages' — schema field name must match Config struct JSON tag")
	} else {
		aamObj, ok := aamProp.(map[string]interface{})
		if !ok {
			t.Error("apply_to_assistant_messages schema definition is not an object")
		} else if aamObj["type"] != "boolean" {
			t.Errorf("apply_to_assistant_messages schema type = %v, want 'boolean'", aamObj["type"])
		}
	}

	// Verify additionalProperties: false is still present on the rtk config block.
	configObj, ok := rtkThen["properties"].(map[string]interface{})["config"].(map[string]interface{})
	if !ok {
		t.Fatal("rtk then branch missing config object")
	}
	if ap, hasAP := configObj["additionalProperties"]; !hasAP {
		t.Error("rtk config block missing 'additionalProperties' — must be false")
	} else if apVal, ok := ap.(bool); !ok || apVal {
		t.Errorf("rtk config block additionalProperties = %v, want false", ap)
	}
}