package configui

import (
	"errors"
	"fmt"
	"sync"

	"github.com/zalando/go-keyring"

	"github.com/matteo-psnt/termwise/internal/config"
)

// keychainService must match the service config stores keys under; config
// exports a setter but no getter or delete, which reverting needs.
const keychainService = "termwise"

// The keychain operations, as variables so tests can swap in fakes rather
// than touch the real keychain.
var (
	keychainGet    = func(entry string) (string, error) { return keyring.Get(keychainService, entry) }
	keychainSet    = config.StoreKeychainEntry
	keychainDelete = func(entry string) error { return keyring.Delete(keychainService, entry) }
)

// keychainPrev is what a keychain entry held before the session first wrote
// to it.
type keychainPrev struct {
	value  string
	exists bool
}

// keychainJournal records what each keychain entry held before an editor
// session wrote to it, so reverting the session can put it back. A verified
// key is written as soon as it is accepted, long before the config is saved,
// so without this, reverting the config would leave the old, working key
// replaced. A nil journal writes without recording, as the first-run wizard
// has nothing to revert.
//
// Writes happen in tea.Cmd goroutines, hence the mutex.
type keychainJournal struct {
	mu   sync.Mutex
	prev map[string]keychainPrev
}

func newKeychainJournal() *keychainJournal {
	return &keychainJournal{prev: map[string]keychainPrev{}}
}

// store writes apiKey to providerName's default entry, first recording what
// the entry held unless the session has already written to it. If the old
// value can't be read the key isn't written, since it couldn't be undone.
func (j *keychainJournal) store(providerName, apiKey string) error {
	entry := config.DefaultKeychainEntry(providerName)
	if j == nil {
		return keychainSet(entry, apiKey)
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if _, seen := j.prev[entry]; !seen {
		old, err := keychainGet(entry)
		switch {
		case err == nil:
			j.prev[entry] = keychainPrev{value: old, exists: true}
		case errors.Is(err, keyring.ErrNotFound):
			j.prev[entry] = keychainPrev{}
		default:
			return fmt.Errorf("reading the current key: %w", err)
		}
	}
	return keychainSet(entry, apiKey)
}

// restore puts every entry the session wrote back as it was, deleting the
// ones that didn't exist, and forgets them.
func (j *keychainJournal) restore() error {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	var errs []error
	for entry, prev := range j.prev {
		var err error
		if prev.exists {
			err = keychainSet(entry, prev.value)
		} else if err = keychainDelete(entry); errors.Is(err, keyring.ErrNotFound) {
			err = nil
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", entry, err))
		}
	}
	clear(j.prev)
	return errors.Join(errs...)
}
