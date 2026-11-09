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

func TestExpiring(t *testing.T) {
	var b bot
	called := 0
	var fn stateFn
	fn = func(*echotron.Update) stateFn { called++; return fn }
	msg := &echotron.Update{Message: &echotron.Message{Text: "ciao"}}

	// Before the deadline fn runs and stays wrapped.
	next := b.expiring(time.Now().Add(time.Hour), fn)(msg)
	if called != 1 || next == nil {
		t.Fatalf("before deadline: called=%d next=%v", called, next != nil)
	}
	// After the deadline fn is skipped and the state ends.
	if next := b.expiring(time.Now().Add(-time.Second), fn)(msg); called != 1 || next != nil {
		t.Fatalf("after deadline: called=%d next=%v", called, next != nil)
	}
}
