package utils

import (
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
)

// sseEnterpriseIdents are the enterprise SSEReaderFactory seam identifiers
// that dev.core task 2.3 deletes from sse.go (design.md "后端 5"): the
// SSEReaderFactory type and the BifrostContextKeySSEReaderFactory lookup that
// replaced the default bufio.Scanner reader.
var sseEnterpriseIdents = []string{
	"SSEReaderFactory",
	"BifrostContextKeySSEReaderFactory",
}

// sseEnterpriseIdentsPresent returns the subset of enterprise identifiers still
// present in the sse.go source.
func sseEnterpriseIdentsPresent(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile("sse.go")
	if err != nil {
		t.Fatalf("read core/providers/utils/sse.go: %v", err)
	}
	var found []string
	for _, id := range sseEnterpriseIdents {
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(id) + `\b`)
		if re.MatchString(string(data)) {
			found = append(found, id)
		}
	}
	return found
}

// TestSSEReaderFactoryInjectionRemoved asserts the SSEReaderFactory type and
// its BifrostContextKeySSEReaderFactory injection seam have been deleted from
// core/providers/utils/sse.go (dev.core task 2.3). The sse parsing path then
// always falls back to the default bufio.Scanner-based readers.
//
// TDD red phase: the factory still exists in sse.go (lines 52-82), so this
// FAILS. It will PASS once dev.core deletes the injection logic.
func TestSSEReaderFactoryInjectionRemoved(t *testing.T) {
	if found := sseEnterpriseIdentsPresent(t); len(found) > 0 {
		t.Fatalf("SSEReaderFactory injection logic still present in core/providers/utils/sse.go: %v", found)
	}
}

// TestSSEReadersFallBackToDefaultScanner asserts that with the enterprise
// factory seam removed, GetSSEDataReader / GetSSEEventReader always return the
// default bufio.Scanner-backed readers (Format A data lines + Format B events).
//
// TDD red phase: gated on the factory being absent first, so this FAILS now.
func TestSSEReadersFallBackToDefaultScanner(t *testing.T) {
	// Gate: factory seam must be removed first.
	if found := sseEnterpriseIdentsPresent(t); len(found) > 0 {
		t.Fatalf("SSEReaderFactory injection logic still present in core/providers/utils/sse.go: %v", found)
	}

	// Format A (data-only): OpenAI-style stream parsed via the default reader.
	stream := "data: {\"a\":1}\n" +
		": comment\n" +
		"\n" +
		"data: {\"b\":2}\n" +
		"data: [DONE]\n"
	reader := GetSSEDataReader(nil, strings.NewReader(stream))
	payloads := drainSSEDataReader(t, reader)
	if len(payloads) != 2 || payloads[0] != `{"a":1}` || payloads[1] != `{"b":2}` {
		t.Errorf("GetSSEDataReader default fallback returned unexpected payloads: %#v", payloads)
	}
	if !SSEStreamEndedOnMarker(reader) {
		t.Error("expected SSEStreamEndedOnMarker to be true after a [DONE] terminated stream")
	}

	// Format B (event+data): Anthropic-style event parsed via the default reader.
	eventStream := "event: message_start\n" +
		"data: {\"type\":\"message_start\"}\n" +
		"\n" +
		"event: content_block_stop\n" +
		"data: {\"type\":\"content_block_stop\"}\n" +
		"\n"
	ev := GetSSEEventReader(nil, strings.NewReader(eventStream))
	gotType, gotData, err := ev.ReadEvent()
	if err != nil {
		t.Fatalf("GetSSEEventReader default fallback error: %v", err)
	}
	if gotType != "message_start" || string(gotData) != `{"type":"message_start"}` {
		t.Errorf("GetSSEEventReader event = (%q, %q), want (\"message_start\", %q)", gotType, string(gotData), `{"type":"message_start"}`)
	}

	// Both readers must terminate with io.EOF like any plain stream.
	// The stream carries two events, so drain the second one before EOF.
	if secondType, secondData, err := ev.ReadEvent(); err != nil {
		t.Errorf("GetSSEEventReader second read error: %v", err)
	} else if secondType != "content_block_stop" || string(secondData) != `{"type":"content_block_stop"}` {
		t.Errorf("GetSSEEventReader second event = (%q, %q), want (\"content_block_stop\", %q)", secondType, string(secondData), `{"type":"content_block_stop"}`)
	}
	if _, _, err := ev.ReadEvent(); err != io.EOF {
		t.Errorf("GetSSEEventReader final read = %v, want io.EOF", err)
	}
}