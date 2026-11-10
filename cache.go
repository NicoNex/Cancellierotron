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
}

var db katalis.DB[int64, session]

// save persists the session; callers must hold b.mu.
func (b *bot) save() {
	if err := db.Put(b.chatID, b.session); err != nil {
		log.Println("b.save", "db.Put", err)
	}
}

// addCandidate returns false if msgID is not the open candidacy message.
func (b *bot) addCandidate(msgID int, id int64, name string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if msgID == 0 || msgID != b.CandidacyMsg {
		return false
	}
	b.Candidates[id] = name
	b.save()
	return true
}

// removeCandidate returns false if msgID is not the open candidacy message.
func (b *bot) removeCandidate(msgID int, id int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if msgID == 0 || msgID != b.CandidacyMsg {
		return false
	}
	delete(b.Candidates, id)
	b.save()
	return true
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

func (b *bot) setPlace(place string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Place = place
	b.save()
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
