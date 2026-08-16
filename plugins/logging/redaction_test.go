package logging

import (
	"context"
	"testing"
	"time"

	"github.com/pin-gou/pg-gateway/core/schemas"
	"github.com/pin-gou/pg-gateway/framework/logstore"
	"github.com/stretchr/testify/assert"
)

// TestAttachLogRedactionDataCopiesContextValue verifies async log entries stay
// redaction-free in the OSS build (SetRedactionDataOnContext is a no-op).
func TestAttachLogRedactionDataCopiesContextValue(t *testing.T) {
	ctx := schemas.NewBifrostContext(context.Background(), time.Time{})
	if schemas.SetRedactionDataOnContext(ctx, schemas.RedactionData{
		LiteralReplacements: schemas.RedactionMapsByPhase{
			Input:  map[string]string{"alex_rivera@gmail.com": "[EMAIL-1]"},
			Output: map[string]string{"rivera@example.com": "[EMAIL-2]"},
		},
		ReversibleMappings: schemas.RedactionMapsByPhase{
			Input: map[string]string{"EMAIL-1": "alex_rivera@gmail.com"},
		},
	}) {
		t.Fatal("expected SetRedactionDataOnContext to be a no-op in OSS")
	}
	entry := &logstore.Log{}

	attachLogRedactionData(ctx, entry, true)

	assert.Nil(t, entry.RedactionData)
}

// TestAttachLogRedactionDataClonesContextMaps verifies async log entries never
// accumulate redaction maps in the OSS build.
func TestAttachLogRedactionDataClonesContextMaps(t *testing.T) {
	ctx := schemas.NewBifrostContext(context.Background(), time.Time{})
	reversibleMappings := map[string]string{"EMAIL-1": "alex_rivera@gmail.com"}
	inputLiteralReplacements := map[string]string{"alex_rivera@gmail.com": "[EMAIL-1]"}
	outputLiteralReplacements := map[string]string{"rivera@example.com": "[EMAIL-2]"}
	schemas.SetRedactionDataOnContext(ctx, schemas.RedactionData{
		LiteralReplacements: schemas.RedactionMapsByPhase{
			Input:  inputLiteralReplacements,
			Output: outputLiteralReplacements,
		},
		ReversibleMappings: schemas.RedactionMapsByPhase{
			Input: reversibleMappings,
		},
	})
	entry := &logstore.Log{}

	attachLogRedactionData(ctx, entry, true)
	reversibleMappings["EMAIL-1"] = "mutated@example.com"
	inputLiteralReplacements["alex_rivera@gmail.com"] = "[MUTATED]"
	outputLiteralReplacements["rivera@example.com"] = "[MUTATED]"

	assert.Nil(t, entry.RedactionData)
}

// TestAttachLogRedactionDataSkipsDisabledContentLogging verifies disabled content logging drops sensitive data.
func TestAttachLogRedactionDataSkipsDisabledContentLogging(t *testing.T) {
	ctx := schemas.NewBifrostContext(context.Background(), time.Time{})
	schemas.SetRedactionDataOnContext(ctx, schemas.RedactionData{
		ReversibleMappings: schemas.RedactionMapsByPhase{
			Input: map[string]string{"EMAIL-1": "alex_rivera@gmail.com"},
		},
	})
	entry := &logstore.Log{}

	attachLogRedactionData(ctx, entry, false)

	assert.Nil(t, entry.RedactionData)
}

// TestAttachLogRedactionDataIgnoresMissingContext verifies nil inputs are safe for processing callbacks.
func TestAttachLogRedactionDataIgnoresMissingContext(t *testing.T) {
	entry := &logstore.Log{}

	attachLogRedactionData(nil, entry, true)
	attachLogRedactionData(schemas.NewBifrostContext(context.Background(), time.Time{}), entry, true)

	assert.Nil(t, entry.RedactionData)
}

// TestAttachMCPLogRedactionDataCopiesContextValue verifies MCP entries never
// accumulate redaction snapshots in the OSS build (SetRedactionDataOnContext is a no-op).
func TestAttachMCPLogRedactionDataCopiesContextValue(t *testing.T) {
	ctx := schemas.NewBifrostContext(context.Background(), time.Time{})
	reversibleMappings := map[string]string{"EMAIL-1": "alex_rivera@gmail.com"}
	schemas.SetRedactionDataOnContext(ctx, schemas.RedactionData{
		ReversibleMappings: schemas.RedactionMapsByPhase{Input: reversibleMappings},
	})
	entry := &logstore.MCPToolLog{}

	attachMCPLogRedactionData(ctx, entry, true)
	reversibleMappings["EMAIL-1"] = "mutated@example.com"

	assert.Nil(t, entry.RedactionData)
}

// TestAttachMCPLogRedactionDataSkipsUnavailableContent verifies disabled logging and missing inputs never attach sensitive data.
func TestAttachMCPLogRedactionDataSkipsUnavailableContent(t *testing.T) {
	ctx := schemas.NewBifrostContext(context.Background(), time.Time{})
	schemas.SetRedactionDataOnContext(ctx, schemas.RedactionData{
		ReversibleMappings: schemas.RedactionMapsByPhase{Input: map[string]string{"EMAIL-1": "alex_rivera@gmail.com"}},
	})
	entry := &logstore.MCPToolLog{}

	attachMCPLogRedactionData(ctx, entry, false)
	attachMCPLogRedactionData(nil, entry, true)
	attachMCPLogRedactionData(ctx, nil, true)

	assert.Nil(t, entry.RedactionData)
}
