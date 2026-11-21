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
	// PollMsg is the ID of the pinned meeting poll; 0 means none.
	PollMsg int
	// PollID is the Telegram ID of the attendance poll, used to match the votes.
	PollID    string
	Attendees map[int64]string // user ID -> display name of who said they come
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

// swapPoll stores msgID and pollID as the attendance poll, resets the attendees
// and returns the previous poll message ID.
func (b *bot) swapPoll(msgID int, pollID string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	old := b.PollMsg
	polls.Delete(b.PollID)
	if pollID != "" {
		polls.Store(pollID, b)
	}
	b.PollMsg, b.PollID = msgID, pollID
	b.Attendees = make(map[int64]string)
	b.save()
	return old
}

// vote records whether the user comes, if pollID is the current attendance poll.
func (b *bot) vote(pollID string, id int64, name string, coming bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if pollID != b.PollID {
		return
	}
	if coming {
		b.Attendees[id] = name
	} else {
		delete(b.Attendees, id)
	}
	b.save()
}

// attendees returns a copy of who said they come; polled is false if no poll is open.
func (b *bot) attendees() (who map[int64]string, polled bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return maps.Clone(b.Attendees), b.PollID != ""
}

func (b *bot) setPlace(place string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Place = place
	b.save()
}

func (b *bot) meetingPlace() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Place
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
