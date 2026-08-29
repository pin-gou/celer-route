package server

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/pin-gou/celer-route/transports/celer-route-http/lib"
)

// rtkSchemaConfigBlock navigates a loaded config.schema.json object to the rtk
// plugin's config block (allOf → if: name=rtk → then: properties.config).
// It mirrors the navigation used by TestRTKPluginConfig_SchemaContract.
func rtkSchemaConfigBlock(t *testing.T, schemaObj map[string]any) map[string]any {
	t.Helper()
	plugins, ok := schemaObj["properties"].(map[string]any)["plugins"].(map[string]any)
	if !ok {
		t.Fatal("schema missing plugins property")
	}
	items, ok := plugins["items"].(map[string]any)
	if !ok {
		t.Fatal("schema plugins missing items")
	}
	allOf, ok := items["allOf"].([]any)
	if !ok {
		t.Fatal("schema plugins.items missing allOf")
	}
	var rtkThen map[string]any
	for _, branch := range allOf {
		b, ok := branch.(map[string]any)
		if !ok {
			continue
		}
		ifBlock, ok := b["if"].(map[string]any)
		if !ok {
			continue
		}
		props, ok := ifBlock["properties"].(map[string]any)
		if !ok {
			continue
		}
		nameProp, ok := props["name"].(map[string]any)
		if !ok {
			continue
		}
		if nameProp["const"] == "rtk" {
			rtkThen, ok = b["then"].(map[string]any)
			break
		}
	}
	if rtkThen == nil {
		t.Fatal("schema missing rtk if/then branch")
	}
	configBlock, ok := rtkThen["properties"].(map[string]any)["config"].(map[string]any)
	if !ok {
		t.Fatal("rtk then branch missing config block")
	}
	return configBlock
}

// TestConfigSchema_RTK_InjectFetchTool_Field verifies that the rtk block of
// config.schema.json declares the inject_fetch_tool field as an optional
// boolean defaulting to true (design.md §5), and that the schema itself
// remains valid JSON with a $schema declaration after the addition.
//
// Red phase: inject_fetch_tool is absent from the schema today, so the field
// assertion fails until dev.transports task 7.1 adds it.
func TestConfigSchema_RTK_InjectFetchTool_Field(t *testing.T) {
	schemaBytes, err := os.ReadFile(getSchemaPath(t))
	if err != nil {
		t.Fatalf("failed to read config.schema.json: %v", err)
	}

	// JSON Schema itself must remain legal: parseable JSON with a $schema
	// declaration (guards against a malformed edit during the dev phase).
	var schemaObj map[string]any
	if err := json.Unmarshal(schemaBytes, &schemaObj); err != nil {
		t.Fatalf("config.schema.json is not valid JSON: %v", err)
	}
	if _, ok := schemaObj["$schema"]; !ok {
		t.Fatal("config.schema.json missing $schema declaration")
	}

	configBlock := rtkSchemaConfigBlock(t, schemaObj)
	configProps, ok := configBlock["properties"].(map[string]any)
	if !ok {
		t.Fatal("rtk config block missing properties")
	}

	field, ok := configProps["inject_fetch_tool"]
	if !ok {
		t.Fatal("rtk config block missing inject_fetch_tool field (design.md §5)")
	}
	fieldObj, ok := field.(map[string]any)
	if !ok {
		t.Fatalf("inject_fetch_tool schema entry has unexpected shape: %T", field)
	}
	if fieldObj["type"] != "boolean" {
		t.Errorf("inject_fetch_tool type = %v, want boolean", fieldObj["type"])
	}
	if fieldObj["default"] != true {
		t.Errorf("inject_fetch_tool default = %v, want true", fieldObj["default"])
	}
}

// TestConfigSchema_RTK_InjectFetchTool_RequiredUnchanged verifies that adding
// inject_fetch_tool does not disturb the rtk block's required constraints:
//
//  1. the config block must not gain a required entry (inject_fetch_tool is
//     optional — it carries a default),
//  2. an existing canonical rtk config (without inject_fetch_tool) must still
//     pass schema validation, and
//  3. a config that sets inject_fetch_tool must now be accepted — today
//     additionalProperties:false rejects the unknown key, which is the red
//     phase driver for this test.
func TestConfigSchema_RTK_InjectFetchTool_RequiredUnchanged(t *testing.T) {
	schemaBytes, err := os.ReadFile(getSchemaPath(t))
	if err != nil {
		t.Fatalf("failed to read config.schema.json: %v", err)
	}
	var schemaObj map[string]any
	if err := json.Unmarshal(schemaBytes, &schemaObj); err != nil {
		t.Fatalf("config.schema.json is not valid JSON: %v", err)
	}
	configBlock := rtkSchemaConfigBlock(t, schemaObj)

	// 1. inject_fetch_tool must NOT be required (it is a defaulted field).
	if req, exists := configBlock["required"]; exists {
		t.Fatalf("rtk config block must not gain a required constraint on inject_fetch_tool; got required: %v", req)
	}

	// 2. Existing canonical rtk config (no inject_fetch_tool) still validates.
	existing := rtkPluginConfig(`{
		"enabled": true,
		"intensity": "standard",
		"max_lines_per_result": 120
	}`)
	if err := lib.ValidateConfigSchema([]byte(existing), schemaBytes); err != nil {
		t.Errorf("existing rtk config should still validate after adding inject_fetch_tool: %v", err)
	}

	// 3. A config that sets inject_fetch_tool must now validate (red phase:
	//    additionalProperties:false rejects the unknown key today).
	withTool := rtkPluginConfig(`{
		"enabled": true,
		"intensity": "standard",
		"inject_fetch_tool": false
	}`)
	if err := lib.ValidateConfigSchema([]byte(withTool), schemaBytes); err != nil {
		t.Errorf("rtk config with inject_fetch_tool should validate once the schema adds the field: %v", err)
	}
}
