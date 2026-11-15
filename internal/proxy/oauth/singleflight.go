package oauth

import (
	"sync"
)

// Single-flight refresh coordination.
//
// WHY THIS EXISTS
// Refresh tokens are single-use at most providers: once a refresh token is
// exchanged, the old one is invalidated. If two goroutines refresh the SAME
// connection concurrently — e.g. a chat request and a "Test Connection" click
// landing in the same second — both send the same refresh token, the loser gets
// `refresh_token_reused` / `invalid_grant`, and a healthy account is bricked
// until the user signs in again.
//
// This file provides a per-connection slot: a mutex that serialises the whole
// read → refresh → persist sequence, plus a generation counter so a caller
// that waited learns that someone else completed the work while it queued.
//
// The slot is per connection, not global: refreshing account A never blocks
// account B.
type RefreshSlot struct {
	Mu sync.Mutex

	coordMu sync.Mutex
	epoch   uint64
	active  int
	// Token is the access token produced during the active concurrent wave.
	Token string
	// Err is the outcome of the active concurrent wave.
	Err error
}

var (
	refreshSlotsMu sync.Mutex
	refreshSlots   = map[string]*RefreshSlot{}
)

// SlotFor returns the refresh slot for a connection. The map holds one small
// struct per connection ID and is never pruned; the key space is bounded by the
// number of connections an operator actually has, which is small (dozens, not
// millions), so the memory cost is negligible.
func SlotFor(connectionID string) *RefreshSlot {
	refreshSlotsMu.Lock()
	defer refreshSlotsMu.Unlock()
	slot, ok := refreshSlots[connectionID]
	if !ok {
		slot = &RefreshSlot{}
		refreshSlots[connectionID] = slot
	}
	return slot
}

// WithRefreshLock runs fn while holding the per-connection refresh lock.
//
// Unlike singleflight.Group, this does not collapse concurrent callers into
// one: every caller runs its own fn body. Only the actual refresh is
// serialised, which is what lets each caller keep its own credential when no
// refresh turns out to be needed.
func WithRefreshLock(connectionID string, fn func() error) error {
	if connectionID == "" {
		// Nothing to key on: a caller without a connection id is already
		// outside the shared-credential path, so run unlocked rather than
		// serialising every such call on one empty-string key.
		return fn()
	}
	slot := SlotFor(connectionID)
	slot.Mu.Lock()
	defer slot.Mu.Unlock()
	return fn()
}

// RefreshOutcome is what a caller learns from a refresh attempt: the fresh
// token on success, or the error that stopped it.
type RefreshOutcome struct {
	Token string
	Err   error
	// Led says whether this caller performed the exchange itself.
	Led bool
}

// RunRefresh coordinates one refresh attempt across concurrent callers for the
// same connection.
//
// needsFn is evaluated inside the lock, so a caller that waited for the winner
// sees the winner's write and reports false — that is the whole point of the
// re-check. When it reports false, RunRefresh returns the winner's outcome:
// the fresh token on success, or the winner's error (each caller keeps its own
// credential on failure).
//
// refreshFn performs the exchange and persists the result. It runs at most
// once per generation.
func RunRefresh(connectionID string, needsFn func() (bool, error), refreshFn func() (string, error)) RefreshOutcome {
	if connectionID == "" {
		needs, err := needsFn()
		if err != nil || !needs {
			return RefreshOutcome{Err: err, Led: true}
		}
		token, rerr := refreshFn()
		return RefreshOutcome{Token: token, Err: rerr, Led: true}
	}

	slot := SlotFor(connectionID)
	slot.coordMu.Lock()
	arrivalEpoch := slot.epoch
	slot.active++
	slot.coordMu.Unlock()
	defer func() {
		slot.coordMu.Lock()
		slot.active--
		if slot.active == 0 {
			// Clear wave outcome once every concurrent waiter leaves so later
			// expiry cycles and transient-failure retries always re-evaluate.
			slot.Token = ""
			slot.Err = nil
		}
		slot.coordMu.Unlock()
	}()

	slot.Mu.Lock()
	defer slot.Mu.Unlock()

	slot.coordMu.Lock()
	if slot.epoch != arrivalEpoch && (slot.Token != "" || slot.Err != nil) {
		tok, err := slot.Token, slot.Err
		slot.coordMu.Unlock()
		// Someone completed an exchange while we queued. On success hand back
		// the fresh token; on failure hand back the error and let the caller
		// keep its own credential.
		return RefreshOutcome{Token: tok, Err: err}
	}
	slot.coordMu.Unlock()

	needs, err := needsFn()
	if err != nil || !needs {
		// Token is already fresh or this caller arrived with a valid credential.
		return RefreshOutcome{Err: err, Led: true}
	}
	token, rerr := refreshFn()
	slot.coordMu.Lock()
	slot.epoch++
	slot.Token = token
	slot.Err = rerr
	slot.coordMu.Unlock()
	return RefreshOutcome{Token: token, Err: rerr, Led: true}
}

// ResetRefreshSlots clears the per-connection refresh slots. Test-only.
func ResetRefreshSlots() {
	refreshSlotsMu.Lock()
	defer refreshSlotsMu.Unlock()
	refreshSlots = make(map[string]*RefreshSlot)
}
