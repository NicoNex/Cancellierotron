package main

import (
	_ "embed"
	"flag"
	"fmt"
	"html"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
	_ "time/tzdata" // Europe/Rome must load even on hosts without zoneinfo.

	"github.com/NicoNex/echotron/v3"
	"github.com/NicoNex/safemap"
)

var (
	//go:embed TOKEN
	token string

	rome *time.Location

	// stateTimeout is how long a pending operation (e.g. /luogo) waits for the reply.
	stateTimeout = 5 * time.Minute

	commands = []echotron.BotCommand{
		{Command: "/annulla", Description: "Annulla l'incontro di sabato (solo admin)."},
		{Command: "/luogo", Description: "Imposta il luogo dell'incontro (solo admin)."},
		{Command: "/cancella_luogo", Description: "Cancella il luogo dell'incontro (solo admin)."},
	}
)

type stateFn func(*echotron.Update) stateFn

// pending is a user's ongoing operation, dropped once deadline passes.
type pending struct {
	fn       stateFn
	deadline time.Time
}

type bot struct {
	chatID int64
	// usrstate holds the pending operation of each user; Update runs concurrently.
	usrstate safemap.Map[int64, pending]
	mu       sync.Mutex // guards session: Update and tick run concurrently
	session
	echotron.API
}

func (b *bot) messagef(f string, a ...any) {
	if _, err := b.SendMessage(fmt.Sprintf(f, a...), b.chatID, nil); err != nil {
		log.Println("b.messagef", "b.SendMessage", err)
	}
}

func newBot(chatID int64) echotron.Bot {
	b := &bot{
		chatID: chatID,
		API:    echotron.NewAPI(token),
	}

	// katalis.Get fails (EOF) on a missing key, so check existence first.
	if ok, err := db.Has(chatID); err != nil {
		log.Println("newBot", "db.Has", err)
	} else if ok {
		if b.session, err = db.Get(chatID); err != nil {
			log.Println("newBot", "db.Get", err)
		}
	} else {
		// Persist new chats so they are restored at the next startup.
		defer b.save()
	}
	if b.Candidates == nil {
		b.Candidates = make(map[int64]string)
	}
	go b.tick()

	return b
}

func (b *bot) Update(update *echotron.Update) {
	if update.CallbackQuery != nil {
		b.handleCallback(update.CallbackQuery)
		return
	}
	if update.Message == nil {
		return
	}

	uid := userID(update)
	// LoadAndDelete keeps two quick messages from running the same state twice.
	if p, ok := b.usrstate.LoadAndDelete(uid); ok && time.Now().Before(p.deadline) {
		if next := p.fn(update); next != nil {
			b.usrstate.Store(uid, pending{next, p.deadline})
		}
		return
	}
	if next := b.handleMessage(update); next != nil {
		b.usrstate.Store(uid, pending{next, time.Now().Add(stateTimeout)})
	}
}

// command returns the command in msg without the "@botname" suffix, or "".
func command(msg string) string {
	word, _, _ := strings.Cut(msg, " ")
	cmd, _, _ := strings.Cut(word, "@")
	return cmd
}

func (b *bot) handleMessage(update *echotron.Update) stateFn {
	cmd := command(update.Message.Text)
	switch cmd {
	case "/annulla", "/luogo", "/cancella_luogo":
	default:
		return nil
	}
	// Every command is admin-only; check once, after filtering out plain chat.
	if !b.isAdmin(userID(update)) {
		return nil
	}

	switch cmd {
	case "/annulla":
		b.cancelMeeting()

	case "/cancella_luogo":
		b.setPlace("")
		b.messagef("Il luogo dell'incontro è stato rimosso.")

	case "/luogo":
		_, err := b.SendMessage(
			"Indica il luogo dell'incontro rispondendo a questo messaggio.\nPer annullare invia /annulla.",
			b.chatID,
			&echotron.MessageOptions{
				ReplyParameters: echotron.ReplyParameters{MessageID: update.Message.ID},
				ReplyMarkup: echotron.ForceReply{
					ForceReply:            true,
					Selective:             true,
					InputFieldPlaceholder: "Luogo dell'incontro",
				},
			},
		)
		if err != nil {
			log.Println("b.handleMessage", "b.SendMessage", err)
			return nil
		}
		return b.setPlaceState
	}

	return nil
}

func (b *bot) setPlaceState(update *echotron.Update) stateFn {
	switch msg := strings.TrimSpace(update.Message.Text); {
	// Any command aborts, so it is never saved as the place.
	case strings.HasPrefix(msg, "/"):
		b.messagef("Operazione annullata.")
		return nil

	case msg == "":
		b.messagef("Il luogo deve essere un messaggio di testo. Riprova.")
		return b.setPlaceState

	default:
		b.setPlace(msg)
		b.messagef("Luogo dell'incontro impostato: %s", msg)
		return nil
	}
}

func (b *bot) handleCallback(q *echotron.CallbackQuery) {
	var text string

	switch q.Data {
	case "candidati":
		b.addCandidate(q.From.ID, q.From.FirstName)
		text = "Candidatura registrata."
	case "ritira":
		b.removeCandidate(q.From.ID)
		text = "Candidatura ritirata."
	}

	_, err := b.AnswerCallbackQuery(q.ID, &echotron.CallbackQueryOptions{Text: text})
	if err != nil {
		log.Println("b.handleCallback", "b.AnswerCallbackQuery", err)
	}
}

func (b *bot) cancelMeeting() {
	day := nextMeeting(time.Now().In(rome))
	b.setSkipDate(day.Format(time.DateOnly))
	b.messagef("L'incontro di sabato %s è annullato.", day.Format("02/01"))
}

// nextMeeting returns the date of the next Saturday meeting (18:30) not yet started.
func nextMeeting(now time.Time) time.Time {
	d := (int(time.Saturday) - int(now.Weekday()) + 7) % 7
	if d == 0 && now.Hour()*60+now.Minute() >= 18*60+30 {
		d = 7
	}
	return now.AddDate(0, 0, d)
}

func (b *bot) sendCandidacy() {
	b.clearCandidates()

	_, err := b.SendMessage(
		"📚 Sono aperte le candidature per scegliere il libro del mese.\nChi desidera proporsi può farlo entro il giorno 1: il nome verrà estratto a sorte tra i candidati.",
		b.chatID,
		&echotron.MessageOptions{
			ReplyMarkup: echotron.InlineKeyboardMarkup{
				InlineKeyboard: [][]echotron.InlineKeyboardButton{
					{
						{Text: "Mi candido", CallbackData: "candidati"},
						{Text: "Ritiro la candidatura", CallbackData: "ritira"},
					},
				},
			},
		},
	)
	if err != nil {
		log.Println("b.sendCandidacy", "b.SendMessage", err)
	}
}

func (b *bot) drawPicker() {
	id, name, ok := b.popRandomCandidate()
	if !ok {
		b.messagef("Questo mese non ci sono state candidature per la scelta del libro.")
		return
	}

	_, err := b.SendMessage(
		fmt.Sprintf(
			`📚 È stato estratto/a <a href="tg://user?id=%d">%s</a>, a cui spetta la scelta del libro del mese. Attendiamo il titolo.`,
			id,
			html.EscapeString(name),
		),
		b.chatID,
		&echotron.MessageOptions{ParseMode: echotron.HTML},
	)
	if err != nil {
		log.Println("b.drawPicker", "b.SendMessage", err)
	}
}

func (b *bot) sendReminder() {
	b.mu.Lock()
	place := b.Place
	b.mu.Unlock()

	if place != "" {
		place = "\nLuogo: " + place
	}
	b.messagef("Si ricorda che oggi alle 18:30 si terrà l'incontro.%s", place)

	_, err := b.SendPoll(
		b.chatID,
		"Parteciperai all'incontro di oggi alle 18:30?",
		[]echotron.InputPollOption{{Text: "Ci sono"}, {Text: "Non ci sono"}},
		&echotron.PollOptions{IsAnonymous: false},
	)
	if err != nil {
		log.Println("b.sendReminder", "b.SendPoll", err)
	}
}

// tick runs this chat's scheduled jobs once a day, during the 10 o'clock hour.
func (b *bot) tick() {
	for t := range time.Tick(time.Hour) {
		now := t.In(rome)
		if now.Hour() != 10 {
			continue
		}

		if now.Day() == 25 {
			b.sendCandidacy()
		}
		if now.Day() == 1 {
			b.drawPicker()
		}

		b.mu.Lock()
		skip := b.SkipDate == now.Format(time.DateOnly)
		b.mu.Unlock()
		if now.Weekday() == time.Saturday && !skip {
			b.sendReminder()
		}
	}
}

func (b *bot) isAdmin(id int64) bool {
	res, err := b.GetChatMember(b.chatID, id)
	if err != nil {
		log.Println("b.isAdmin", "b.GetChatMember", err)
		return false
	}
	return res.Result != nil && (res.Result.Status == "creator" || res.Result.Status == "administrator")
}

func userID(u *echotron.Update) int64 {
	switch {
	case u.Message != nil && u.Message.From != nil:
		return u.Message.From.ID
	case u.CallbackQuery != nil:
		return u.CallbackQuery.From.ID
	}

	return 0
}

func main() {
	flag.StringVar(&token, "t", token, "The Telegram token of the bot.")
	flag.Parse()
	// An editor may leave a trailing newline in the TOKEN file.
	token = strings.TrimSpace(token)

	var err error
	if rome, err = time.LoadLocation("Europe/Rome"); err != nil {
		log.Fatal("main", "time.LoadLocation", err)
	}
	openDB()

	// Intercept SIGINT and SIGTERM and gracefully close the DB.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		db.Close()
		os.Exit(0)
	}()

	api := echotron.NewAPI(token)
	if _, err := api.SetMyCommands(nil, commands...); err != nil {
		log.Println("main", "api.SetMyCommands", err)
	}

	dopts := echotron.UpdateOptions{
		AllowedUpdates: []echotron.UpdateType{
			echotron.MessageUpdate,
			echotron.CallbackQueryUpdate,
		},
		Timeout: 120,
	}
	dsp := echotron.NewDispatcher(token, newBot)
	for id := range db.Items() {
		log.Printf("starting dispatcher session with ID: %d", id)
		dsp.AddSession(id)
	}
	// Drop the backlog only at startup; retries after a network error keep it.
	drop := true
	for {
		log.Println(dsp.PollOptions(drop, dopts))
		drop = false
		time.Sleep(5 * time.Second)
	}
}
