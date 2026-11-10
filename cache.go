package main

import (
	"log"
	"maps"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/NicoNex/katalis"
)

// session is the persisted per-chat state. Fields are exported for gob.
type session struct {
	Candidates map[int64]string // user ID -> display name
	SkipDate   string           // date (time.DateOnly) of a cancelled Saturday meeting
	Place      string           // meeting place; empty means not set
	// CandidacyMsg is the ID of the open candidacy message; 0 means closed.
	CandidacyMsg int
	// PollMsg is the ID of the pinned Saturday poll; 0 means none.
	PollMsg int
}

var db katalis.DB[int64, session]

// save persists the session; callers must hold b.mu.
func (b *bot) save() {
	if err := db.Put(b.chatID, b.session); err != nil {
		log.Println("b.save", "db.Put", err)
	}
}

// addCandidate reports whether msgID is the open candidacy message and whether the list changed.
func (b *bot) addCandidate(msgID int, id int64, name string) (open, changed bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if msgID == 0 || msgID != b.CandidacyMsg {
		return false, false
	}
	if old, ok := b.Candidates[id]; ok && old == name {
		return true, false
	}
	b.Candidates[id] = name
	b.save()
	return true, true
}

// removeCandidate reports whether msgID is the open candidacy message and whether the list changed.
func (b *bot) removeCandidate(msgID int, id int64) (open, changed bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if msgID == 0 || msgID != b.CandidacyMsg {
		return false, false
	}
	if _, ok := b.Candidates[id]; !ok {
		return true, false
	}
	delete(b.Candidates, id)
	b.save()
	return true, true
}

// openCandidacy clears the candidates and accepts them only from msgID.
func (b *bot) openCandidacy(msgID int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	clear(b.Candidates)
	b.CandidacyMsg = msgID
	b.save()
}

// closeCandidacy stops accepting candidates and returns the closed message ID.
func (b *bot) closeCandidacy() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	msgID := b.CandidacyMsg
	b.CandidacyMsg = 0
	b.save()
	return msgID
}

func (b *bot) setSkipDate(date string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.SkipDate = date
	b.save()
}

// swapPollMsg stores id as the pinned poll and returns the previous one.
func (b *bot) swapPollMsg(id int) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	old := b.PollMsg
	if old != id {
		b.PollMsg = id
		b.save()
	}
	return old
}

func (b *bot) setPlace(place string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Place = place
	b.save()
}

// candidateNames returns the candidates' names, sorted.
func (b *bot) candidateNames() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Sorted(maps.Values(b.Candidates))
}

// popRandomCandidate picks a random candidate and clears the list; ok is false if empty.
func (b *bot) popRandomCandidate() (id int64, name string, ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ids := slices.Collect(maps.Keys(b.Candidates))
	if len(ids) == 0 {
		return 0, "", false
	}
	id = ids[rand.IntN(len(ids))]
	name = b.Candidates[id]
	clear(b.Candidates)
	b.save()
	return id, name, true
}

// openDB opens the database; called from main so tests never touch real data.
func openDB() {
	cd, err := os.UserCacheDir()
	if err != nil {
		log.Fatal("openDB", "os.UserCacheDir", err)
	}

	db, err = katalis.OpenOptions(
		filepath.Join(cd, "ciambellanotron"),
		katalis.Int64Codec,
		katalis.Gob[session](),
		&katalis.Options{
			// -1 syncs on every write: writes are rare, polling fsync is not.
			BackgroundSyncInterval:       -1,
			BackgroundCompactionInterval: 24 * time.Hour,
		},
	)
	if err != nil {
		log.Fatal("openDB", "katalis.OpenOptions", err)
	}
}
