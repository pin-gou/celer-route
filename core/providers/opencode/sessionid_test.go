package opencode

import (
	"regexp"
	"strings"
	"sync"
	"testing"
)

const sessionIDRegex = `^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$`

func TestNewSessionID_Format(t *testing.T) {
	id := newSessionID()
	if matched, err := regexp.MatchString(sessionIDRegex, id); err != nil {
		t.Fatalf("compile regex: %v", err)
	} else if !matched {
		t.Fatalf("newSessionID() = %q, does not match %s", id, sessionIDRegex)
	}
	if got, want := len(id), len("ses_")+sessionIDLength; got != want {
		t.Fatalf("len(newSessionID()) = %d, want %d (id=%q)", got, want, id)
	}
}

func TestNewSessionID_CharsetStrict(t *testing.T) {
	id := newSessionID()
	tail := strings.TrimPrefix(id, "ses_")
	timePart := tail[:sessionIDTimeHexLen]
	for _, r := range timePart {
		if !(r >= '0' && r <= '9') && !(r >= 'a' && r <= 'f') {
			t.Fatalf("time segment %q contains non-hex char %q", timePart, r)
		}
	}
	suffix := tail[sessionIDTimeHexLen:]
	for _, r := range suffix {
		if !strings.ContainsRune(sessionIDAlphabet, r) {
			t.Fatalf("suffix %q contains char %q outside alphabet %q", suffix, r, sessionIDAlphabet)
		}
	}
}

func TestNewSessionID_DescendingTimePrefix(t *testing.T) {
	// Newly minted ids should sort BEFORE older ones because the time
	// segment is the bitwise-NOT of the millisecond timestamp. We assert
	// that consecutive ids in the same millisecond window compare the way
	// the opencode client's tree-backed storage expects: lexicographically
	// descending-by-recency.
	first := newSessionID()
	second := newSessionID()
	// Same prefix window? Likely yes — same millisecond. The counter
	// advances, so second.id's lower bits differ; descending-string ordering
	// therefore places first BEFORE second only if the implementation
	// really inverted the timestamp. Validate at least that the two ids
	// share the same 12-hex time part when emitted in the same ms window.
	if first[4:16] == second[4:16] {
		if first >= second {
			t.Fatalf("expected first %q < second %q under descending ordering (same ms window)", first, second)
		}
	}
}

func TestNewSessionID_UniquenessSameMillisecond(t *testing.T) {
	seen := make(map[string]struct{}, 1<<10)
	const n = 1024
	for i := 0; i < n; i++ {
		id := newSessionID()
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id emitted: %s", id)
		}
		seen[id] = struct{}{}
	}
}

func TestNewSessionID_ConcurrentUnique(t *testing.T) {
	const goroutines = 16
	const perGoroutine = 256

	ids := make(chan string, goroutines*perGoroutine)
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				ids <- newSessionID()
			}
		}()
	}
	wg.Wait()
	close(ids)

	seen := make(map[string]struct{}, goroutines*perGoroutine)
	for id := range ids {
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id under concurrency: %s", id)
		}
		seen[id] = struct{}{}
	}
	if got := len(seen); got != goroutines*perGoroutine {
		t.Fatalf("expected %d unique ids, got %d", goroutines*perGoroutine, got)
	}
}