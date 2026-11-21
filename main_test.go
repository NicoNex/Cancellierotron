package main

import (
	"strings"
	"testing"
	"time"

	"github.com/NicoNex/echotron/v3"
	"github.com/NicoNex/katalis"
)

func TestNextMeeting(t *testing.T) {
	loc := time.UTC
	cases := []struct{ now, want string }{
		{"2026-11-09 12:00", "2026-11-14"}, // Monday -> same week
		{"2026-11-11 10:00", "2026-11-14"}, // Wednesday poll -> same week
		{"2026-11-14 10:00", "2026-11-14"}, // Saturday morning -> today
		{"2026-11-14 18:30", "2026-11-21"}, // Saturday after start -> next week
		{"2026-11-15 09:00", "2026-11-21"}, // Sunday -> next week
	}
	for _, c := range cases {
		now, _ := time.ParseInLocation("2006-01-02 15:04", c.now, loc)
		if got := nextMeeting(now).Format(time.DateOnly); got != c.want {
			t.Errorf("nextMeeting(%s) = %s, want %s", c.now, got, c.want)
		}
	}
}

func TestPendingExpiry(t *testing.T) {
	var b bot
	called := 0
	var fn stateFn
	fn = func(*echotron.Update) stateFn { called++; return fn }
	msg := &echotron.Update{Message: &echotron.Message{Text: "ciao", From: &echotron.User{ID: 1}}}

	// Before the deadline the state runs and stays pending.
	b.usrstate.Store(1, pending{fn, time.Now().Add(time.Hour)})
	b.Update(msg)
	if _, ok := b.usrstate.Load(1); called != 1 || !ok {
		t.Fatalf("before deadline: called=%d pending=%v", called, ok)
	}

	// After the deadline the state is dropped without running.
	b.usrstate.Store(1, pending{fn, time.Now().Add(-time.Second)})
	b.Update(msg)
	if _, ok := b.usrstate.Load(1); called != 1 || ok {
		t.Fatalf("after deadline: called=%d pending=%v", called, ok)
	}
}

func TestCommand(t *testing.T) {
	for in, want := range map[string]string{
		"/luogo":           "/luogo",
		"/luogo@bot extra": "/luogo",
		"/annullaqualcosa": "/annullaqualcosa",
		"ciao /annulla":    "ciao",
	} {
		if got := command(in); got != want {
			t.Errorf("command(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCandidacyWindow(t *testing.T) {
	var err error
	if db, err = katalis.Open(t.TempDir(), katalis.Int64Codec, katalis.Gob[session]()); err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	b := bot{chatID: 1, session: session{Candidates: map[int64]string{}}}
	if open, _ := b.addCandidate(0, 1, "a"); open {
		t.Fatal("candidacy accepted before opening")
	}
	b.openCandidacy(42)
	if open, _ := b.addCandidate(41, 1, "a"); open {
		t.Fatal("candidacy accepted from an old message")
	}
	for _, c := range []struct {
		id   int64
		name string
	}{{1, "a"}, {2, "Bea"}} {
		if open, changed := b.addCandidate(42, c.id, c.name); !open || !changed {
			t.Fatalf("candidacy %q rejected while open", c.name)
		}
	}
	if _, changed := b.addCandidate(42, 1, "a"); changed {
		t.Fatal("re-adding the same candidate reported a change")
	}
	if got := candidacyText(b.candidateNames()); !strings.HasSuffix(got, "Candidati:\n• Bea\n• a") {
		t.Fatalf("candidacyText = %q", got)
	}
	if _, changed := b.removeCandidate(42, 3); changed {
		t.Fatal("removing a non-candidate reported a change")
	}
	b.removeCandidate(42, 2)
	if got := b.closeCandidacy(); got != 42 {
		t.Fatalf("closeCandidacy = %d, want 42", got)
	}
	if open, _ := b.addCandidate(42, 2, "b"); open {
		t.Fatal("candidacy changed after closing")
	}
	if open, _ := b.removeCandidate(42, 1); open {
		t.Fatal("candidacy changed after closing")
	}
}

func TestAttendance(t *testing.T) {
	var err error
	if db, err = katalis.Open(t.TempDir(), katalis.Int64Codec, katalis.Gob[session]()); err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	b := &bot{chatID: -1}
	b.swapPoll(10, "p1")
	voterRelay{}.Update(&echotron.Update{PollAnswer: &echotron.PollAnswer{PollID: "p1", User: &echotron.User{ID: 1, FirstName: "a"}, OptionIDs: []int{0}}})
	b.vote("p1", 2, "b", true)
	b.vote("old", 3, "c", true)
	b.vote("p1", 2, "b", false)
	if got, polled := b.attendees(); !polled || len(got) != 1 || got[1] != "a" {
		t.Fatalf("attendees = %v, want only a", got)
	}
	if old := b.swapPoll(0, ""); old != 10 {
		t.Fatal("swapPoll did not return the old poll")
	}
	if got, polled := b.attendees(); polled || len(got) != 0 {
		t.Fatal("swapPoll did not reset the poll")
	}
	if _, ok := polls.Load("p1"); ok {
		t.Fatal("old poll still routed")
	}
}
