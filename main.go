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
	"unicode/utf16"

	"github.com/NicoNex/echotron/v3"
	"github.com/NicoNex/safemap"
)

// maxPlaceLen keeps the place within the poll question, which Telegram caps at 300 characters.
const maxPlaceLen = 200

var (
	//go:embed TOKEN
	token string

	rome *time.Location

	// stateTimeout is how long a pending operation (e.g. /luogo) waits for the reply.
	stateTimeout = 5 * time.Minute

	// infoURL is the group info page; a telegra.ph page opens in Telegram's Instant View.
	infoURL = "https://telegra.ph/"

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
	if len(update.Message.NewChatMembers) > 0 {
		b.welcome(update.Message.NewChatMembers)
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
	case "/annulla", "/luogo", "/cancella_luogo", "/debug":
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

	// /debug is left out of commands so it stays hidden from the UI.
	case "/debug":
		var kbd [][]echotron.InlineKeyboardButton
		for _, a := range debugActions {
			kbd = append(kbd, []echotron.InlineKeyboardButton{{Text: a.label, CallbackData: a.data}})
		}
		_, err := b.SendMessage("🛠 Debug: scegli la funzione da eseguire subito.", b.chatID, &echotron.MessageOptions{
			ReplyMarkup: echotron.InlineKeyboardMarkup{InlineKeyboard: kbd},
		})
		if err != nil {
			log.Println("b.handleMessage", "b.SendMessage", err)
		}

	case "/cancella_luogo":
		b.setPlace("")
		b.messagef("Il luogo dell'incontro è stato rimosso.")

	case "/luogo":
		_, err := b.SendMessage(
			"Indica il luogo dell'incontro rispondendo a questo messaggio, con la preposizione davanti (es. \"al Vergnano\").\nPer annullare invia /annulla.",
			b.chatID,
			&echotron.MessageOptions{
				ReplyParameters: echotron.ReplyParameters{MessageID: update.Message.ID},
				ReplyMarkup: echotron.ForceReply{
					ForceReply:            true,
					Selective:             true,
					InputFieldPlaceholder: "al Vergnano",
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

	// Telegram counts length in UTF-16 code units, so emoji count double.
	case len(utf16.Encode([]rune(msg))) > maxPlaceLen:
		b.messagef("Il luogo è troppo lungo (massimo %d caratteri). Riprova.", maxPlaceLen)
		return b.setPlaceState

	default:
		b.setPlace(msg)
		b.messagef("Luogo dell'incontro impostato: %s", msg)
		return nil
	}
}

func (b *bot) handleCallback(q *echotron.CallbackQuery) {
	var msgID int
	if q.Message != nil {
		msgID = q.Message.ID
	}

	text := "Le candidature sono chiuse."
	var open, changed bool
	switch q.Data {
	case "candidati":
		if open, changed = b.addCandidate(msgID, q.From.ID, q.From.FirstName); open {
			text = "Candidatura registrata."
		}
	case "ritira":
		if open, changed = b.removeCandidate(msgID, q.From.ID); changed {
			text = "Candidatura ritirata."
		} else if open {
			text = "Non risultavi tra i candidati."
		}
	default:
		for _, a := range debugActions {
			if a.data != q.Data {
				continue
			}
			// The debug keyboard is visible to the whole group, so recheck here.
			text = "Solo gli admin possono usare il debug."
			if b.isAdmin(q.From.ID) {
				text = "Fatto."
				a.run(b, q)
			}
		}
	}

	// Answer first so the button stops spinning before the edit round trip.
	_, err := b.AnswerCallbackQuery(q.ID, &echotron.CallbackQueryOptions{Text: text})
	if err != nil {
		log.Println("b.handleCallback", "b.AnswerCallbackQuery", err)
	}
	if changed {
		b.updateCandidacy(msgID)
	}
}

// debugActions are the jobs /debug can run on demand; it lists them in this order.
var debugActions = []struct {
	label, data string
	run         func(*bot, *echotron.CallbackQuery)
}{
	{"Apri candidature", "debug_candidature", func(b *bot, _ *echotron.CallbackQuery) { b.sendCandidacy() }},
	{"Estrai candidato", "debug_estrazione", func(b *bot, _ *echotron.CallbackQuery) { b.drawPicker() }},
	{"Promemoria sabato", "debug_promemoria", func(b *bot, _ *echotron.CallbackQuery) { b.sendReminder() }},
	{"Benvenuto", "debug_benvenuto", func(b *bot, q *echotron.CallbackQuery) { b.welcome([]*echotron.User{q.From}) }},
}

// mention is an HTML link to the user's profile.
func mention(id int64, name string) string {
	return fmt.Sprintf(`<a href="tg://user?id=%d">%s</a>`, id, html.EscapeString(name))
}

// welcome greets the new human members with a button to the group info page.
func (b *bot) welcome(users []*echotron.User) {
	var names []string
	for _, u := range users {
		if !u.IsBot {
			names = append(names, mention(u.ID, u.FirstName))
		}
	}
	if len(names) == 0 {
		return
	}

	_, err := b.SendMessage(
		fmt.Sprintf(
			"<b>Ciao %s, benvenuto/a nel gruppo del Caffè Letterario Bari! ☕️</b>\n\n"+
				"Io sono il Cancelliere, il bot del gruppo.\n\n"+
				"Se ti va di rompere il ghiaccio, raccontaci cosa stai leggendo ultimamente o quali sono i tuoi generi preferiti. "+
				"E prima di buttarti nella conversazione, dai una rapida occhiata alle info del gruppo!",
			strings.Join(names, ", "),
		),
		b.chatID,
		&echotron.MessageOptions{
			ParseMode: echotron.HTML,
			ReplyMarkup: echotron.InlineKeyboardMarkup{
				InlineKeyboard: [][]echotron.InlineKeyboardButton{
					{{Text: "ℹ️ Informazioni", URL: infoURL}},
				},
			},
		},
	)
	if err != nil {
		log.Println("b.welcome", "b.SendMessage", err)
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

var candidacyKeyboard = echotron.InlineKeyboardMarkup{
	InlineKeyboard: [][]echotron.InlineKeyboardButton{
		{
			{Text: "Mi candido", CallbackData: "candidati"},
			{Text: "Ritiro la candidatura", CallbackData: "ritira"},
		},
	},
}

// candidacyText is the candidacy message listing the current candidates.
func candidacyText(names []string) string {
	text := "📚 Sono aperte le candidature per scegliere il libro del mese.\nChi desidera proporsi può farlo entro il giorno 1: il nome verrà estratto a sorte tra i candidati."
	if len(names) == 0 {
		return text
	}
	return text + "\n\nCandidati:\n• " + strings.Join(names, "\n• ")
}

// updateCandidacy rewrites the candidacy message with the current candidates.
// shortcut: two near-simultaneous clicks may land out of order and show a stale list until the next click; serialize edits if it shows up.
func (b *bot) updateCandidacy(msgID int) {
	_, err := b.EditMessageText(
		candidacyText(b.candidateNames()),
		echotron.NewMessageID(b.chatID, msgID),
		&echotron.MessageTextOptions{ReplyMarkup: candidacyKeyboard},
	)
	if err != nil {
		log.Println("b.updateCandidacy", "b.EditMessageText", err)
	}
}

func (b *bot) sendCandidacy() {
	res, err := b.SendMessage(
		candidacyText(nil),
		b.chatID,
		&echotron.MessageOptions{ReplyMarkup: candidacyKeyboard},
	)
	if err != nil {
		log.Println("b.sendCandidacy", "b.SendMessage", err)
		return
	}
	b.openCandidacy(res.Result.ID)
}

func (b *bot) drawPicker() {
	// Remove the buttons so the closed candidacy is visible in the chat.
	if msgID := b.closeCandidacy(); msgID != 0 {
		if _, err := b.EditMessageReplyMarkup(echotron.NewMessageID(b.chatID, msgID), nil); err != nil {
			log.Println("b.drawPicker", "b.EditMessageReplyMarkup", err)
		}
	}

	id, name, ok := b.popRandomCandidate()
	if !ok {
		b.messagef("Questo mese non ci sono state candidature per la scelta del libro.")
		return
	}

	_, err := b.SendMessage(
		fmt.Sprintf("📚 È stato estratto/a %s, a cui spetta la scelta del libro del mese. Attendiamo il titolo.", mention(id, name)),
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

	question := "Chi c'è stasera alle 18:30?"
	if place != "" {
		// place carries its own preposition, e.g. "al Vergnano".
		question = "Chi c'è stasera " + place + " alle 18:30?"
	}
	res, err := b.SendPoll(
		b.chatID,
		question,
		[]echotron.InputPollOption{{Text: "Ci sono"}, {Text: "Non ci sono"}},
		&echotron.PollOptions{IsAnonymous: echotron.Ptr(false)},
	)
	if err != nil {
		log.Println("b.sendReminder", "b.SendPoll", err)
		return
	}

	// The poll itself already notified the chat.
	if _, err := b.PinChatMessage(b.chatID, res.Result.ID, &echotron.PinMessageOptions{DisableNotification: true}); err != nil {
		log.Println("b.sendReminder", "b.PinChatMessage", err)
		return
	}
	// Unpinning the previous poll here covers a midnight missed while the bot was down.
	b.unpinPoll(res.Result.ID)
}

// unpinPoll unpins the stored Saturday poll, if any, and stores next in its place.
func (b *bot) unpinPoll(next int) {
	if id := b.swapPollMsg(next); id != 0 && id != next {
		if _, err := b.UnpinChatMessage(b.chatID, &echotron.UnpinMessageOptions{MessageID: id}); err != nil {
			log.Println("b.unpinPoll", "b.UnpinChatMessage", err)
		}
	}
}

// tick runs this chat's scheduled jobs: the poll unpin at 00:00, the rest at 10:00.
func (b *bot) tick() {
	for t := range time.Tick(time.Minute) {
		now := t.In(rome)
		if now.Minute() != 0 {
			continue
		}

		switch now.Hour() {
		case 0:
			b.unpinPoll(0)

		case 10:
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
	flag.StringVar(&infoURL, "i", infoURL, "The URL of the group info page.")
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
