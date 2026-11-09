package main

import (
	"github.com/NicoNex/echotron/v3"
	"testing"
	"time"
)

func TestNextMeeting(t *testing.T) {
	loc := time.UTC
	cases := []struct{ now, want string }{
		{"2026-11-09 12:00", "2026-11-14"}, // Monday -> same week
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
