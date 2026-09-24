package configstore

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrNotFound = errors.New("not found")
var ErrAlreadyExists = errors.New("already exists")

// ErrInvitationNotFound is returned by AcceptInvitationTx when no invitation
// row carries the supplied token. The handler maps it to 410 Gone — the same
// response as an unusable token — so token validity is never an oracle that
// lets a caller enumerate which invite links exist.
var ErrInvitationNotFound = errors.New("invitation not found")

// ErrInvitationNotUsable is returned by AcceptInvitationTx when the token
// exists but can no longer be accepted (already accepted, revoked, or past
// its expiry). Also mapped to 410 Gone.
//
// The check runs INSIDE the transaction, against a row locked FOR UPDATE on
// Postgres, so two concurrent accepts of the same single-use token cannot
// both succeed: the loser observes accepted_at already stamped and returns
// this error instead of creating a duplicate team_members row.
var ErrInvitationNotUsable = errors.New("invitation is no longer valid")

// ErrReconciliationAlreadyApplied is returned by ApplyReconciliationTx when
// the batch has already been applied. Applying is one-way: the correction is
// a multiplicative scale on the datasheet, so a second pass compounds it.
// The handler maps this to 409 Conflict.
//
// The status is re-read INSIDE the transaction under a row lock, which is
// what makes the guard hold when two operators click apply concurrently —
// a handler-level pre-check alone reads a stale `matched` for both callers
// and lets both scale the price book.
var ErrReconciliationAlreadyApplied = errors.New("reconciliation batch is already applied")

// ErrReencryptModeUnsupported is returned by ReencryptPlaintextRows when the
// caller asks for a mode the implementation does not yet support (e.g.
// rotate-key until key versioning lands). The CLI surfaces it as-is.
var ErrReencryptModeUnsupported = errors.New("re-encrypt mode is not supported by this build")

// EncryptionNotConfiguredError is returned by the D9 startup policy when no
// encryption key is configured and the operator has not explicitly opted in
// to plaintext storage. It is fatal: the server refuses to boot rather than
// silently persisting provider keys and session tokens in the clear.
//
// Counts (optional) is the per-table breakdown of rows that would need
// migrating, so the operator can size the re-encrypt run before committing.
type EncryptionNotConfiguredError struct {
	Counts PlaintextRowCounts
}

func (e *EncryptionNotConfiguredError) Error() string {
	base := "refusing to start: no encryption_key is configured and allow_plaintext_storage is not enabled. Set encryption_key in config.json (or BIFROST_ENCRYPTION_KEY), or set allow_plaintext_storage=true (BIFROST_ALLOW_PLAINTEXT_STORAGE) to explicitly accept plaintext storage"
	if len(e.Counts) == 0 {
		return base
	}
	// Sorted table order keeps the message (and any test asserting on it)
	// deterministic — map iteration is randomised in Go.
	tables := make([]string, 0, len(e.Counts))
	for table := range e.Counts {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	parts := make([]string, 0, len(tables))
	for _, table := range tables {
		parts = append(parts, fmt.Sprintf("%s=%d", table, e.Counts[table]))
	}
	return fmt.Sprintf(
		"%s. The database currently holds %d plaintext sensitive rows (%s); after configuring the key, run `celer-route-admin admin re-encrypt --mode=plaintext-to-encrypted --confirm` to migrate them",
		base, sumCounts(e.Counts), strings.Join(parts, ", "),
	)
}

func sumCounts(c PlaintextRowCounts) int64 {
	var total int64
	for _, n := range c {
		total += n
	}
	return total
}

// ErrUnresolvedKeys is returned when one or more keys could not be resolved
type ErrUnresolvedKeys struct {
	Identifiers []string
}

func (e *ErrUnresolvedKeys) Error() string {
	return fmt.Sprintf("could not resolve keys: %s", strings.Join(e.Identifiers, ", "))
}
