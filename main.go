package main

import (
	"embed"
	"encoding/json"
	"flag"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

type App struct {
	store *Store
	pages map[string]*template.Template
	auth  *Auth // nil when no password is configured
}

func main() {
	addr := flag.String("addr", ":8082", "listen address")
	dbPath := flag.String("db", "cards.json", "path to the JSON data file")
	secure := flag.Bool("secure-cookies", false, "mark the login cookie Secure (only when served over HTTPS)")
	flag.Parse()

	store, err := OpenStore(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	app := &App{store: store, pages: loadPages()}
	if pw := os.Getenv("ANKI_PASSWORD"); pw != "" {
		app.auth = NewAuth(pw, *secure)
	} else {
		log.Printf("WARNING: ANKI_PASSWORD is not set, anyone who can reach %s can use the app", *addr)
	}

	log.Printf("Deutsch Karten listening on %s (data: %s)", *addr, *dbPath)
	log.Fatal(http.ListenAndServe(*addr, app.handler()))
}

func (a *App) handler() http.Handler {
	static, _ := fs.Sub(staticFS, "static")
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))
	// Served from the root so its scope covers the whole app.
	sw, _ := fs.ReadFile(static, "sw.js")
	mux.HandleFunc("/sw.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(sw)
	})
	mux.HandleFunc("/", a.home)
	mux.HandleFunc("/study", a.study)
	mux.HandleFunc("/review", a.review)
	mux.HandleFunc("/add", a.add)
	mux.HandleFunc("/import", a.importCards)
	mux.HandleFunc("/cards", a.cards)
	mux.HandleFunc("/edit", a.edit)
	mux.HandleFunc("/delete", a.deleteCard)
	mux.HandleFunc("/export", a.export)
	if a.auth == nil {
		return mux
	}
	mux.HandleFunc("/login", a.login)
	mux.HandleFunc("/logout", a.logout)
	return a.auth.Middleware(mux)
}

func loadPages() map[string]*template.Template {
	funcs := template.FuncMap{
		"interval": humanInterval,
		"umlauts":  func() []string { return []string{"ä", "ö", "ü", "ß", "Ä", "Ö", "Ü", "„", "“"} },
		"due": func(t time.Time) string {
			d := time.Until(t)
			if d <= 0 {
				return "now"
			}
			return "in " + humanInterval(d.Hours()/24)
		},
	}
	pages := map[string]*template.Template{}
	for _, name := range []string{"home", "study", "add", "import", "cards", "edit", "login"} {
		pages[name] = template.Must(template.New("").Funcs(funcs).
			ParseFS(templateFS, "templates/layout.html", "templates/fields.html", "templates/"+name+".html"))
	}
	return pages
}

func (a *App) render(w http.ResponseWriter, page string, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	data["Page"] = page
	data["Auth"] = a.auth != nil
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.pages[page].ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("render %s: %v", page, err)
	}
}

func (a *App) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	stats := a.store.Stats()
	var total DeckStats
	for _, s := range stats {
		total.Total += s.Total
		total.New += s.New
		total.Due += s.Due
	}
	a.render(w, "home", map[string]any{"Stats": stats, "Total": total})
}

type ratingOption struct {
	Value int
	Label string
	Next  string
	Class string
}

func (a *App) study(w http.ResponseWriter, r *http.Request) {
	deck := r.URL.Query().Get("deck")
	dir := r.URL.Query().Get("dir")
	if dir != "en-de" {
		dir = "de-en"
	}
	allowNew := r.URL.Query().Get("new") != "0"
	card := a.store.NextDue(deck, allowNew)
	data := map[string]any{"Deck": deck, "Dir": dir, "Card": card, "AllowNew": allowNew}
	if card != nil {
		var opts []ratingOption
		for _, o := range []struct {
			v            int
			label, class string
		}{{Again, "Again", "again"}, {Hard, "Hard", "hard"}, {Good, "Good", "good"}, {Easy, "Easy", "easy"}} {
			iv, _ := schedule(*card, o.v)
			opts = append(opts, ratingOption{o.v, o.label, humanInterval(iv), o.class})
		}
		data["Ratings"] = opts
		data["Remaining"] = a.remaining(deck, allowNew)
	}
	a.render(w, "study", data)
}

func (a *App) remaining(deck string, allowNew bool) map[string]int {
	var due, nw int
	for _, s := range a.store.Stats() {
		if deck != "" && s.Name != deck {
			continue
		}
		due += s.Due
		if allowNew {
			nw += s.New
		}
	}
	return map[string]int{"Due": due, "New": nw}
}

func (a *App) review(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/study", http.StatusSeeOther)
		return
	}
	id, _ := strconv.Atoi(r.FormValue("id"))
	rating, _ := strconv.Atoi(r.FormValue("rating"))
	if rating < Again || rating > Easy {
		http.Error(w, "invalid rating", http.StatusBadRequest)
		return
	}
	if err := a.store.Update(id, func(c *Card) { applyReview(c, rating) }); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	q := url.Values{}
	q.Set("deck", r.FormValue("deck"))
	q.Set("dir", r.FormValue("dir"))
	q.Set("new", r.FormValue("new"))
	http.Redirect(w, r, "/study?"+q.Encode(), http.StatusSeeOther)
}

func cardFromForm(r *http.Request) Card {
	return Card{
		Deck:    strings.TrimSpace(r.FormValue("deck")),
		Front:   strings.TrimSpace(r.FormValue("front")),
		Back:    strings.TrimSpace(r.FormValue("back")),
		Example: strings.TrimSpace(r.FormValue("example")),
		Notes:   strings.TrimSpace(r.FormValue("notes")),
	}
}

func (a *App) add(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{"Decks": a.store.Decks(), "LastDeck": r.URL.Query().Get("deck")}
	if r.Method == http.MethodPost {
		c := cardFromForm(r)
		if c.Front == "" || c.Back == "" {
			data["Error"] = "German word and translation are both required."
			data["Card"] = c
			data["LastDeck"] = c.Deck
			a.render(w, "add", data)
			return
		}
		if err := a.store.Add(&c); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/add?added="+url.QueryEscape(c.Front)+"&deck="+url.QueryEscape(c.Deck), http.StatusSeeOther)
		return
	}
	data["Added"] = r.URL.Query().Get("added")
	a.render(w, "add", data)
}

// parseImport turns pasted text into cards. Each line is
// "German <sep> translation [<sep> example [<sep> notes]]" where <sep> is
// a tab, ";", "|", " = " or " - ". Blank lines and lines starting with # are skipped.
func parseImport(text, deck string) (cards []*Card, skipped []string) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var parts []string
		for _, sep := range []string{"\t", ";", "|", " = ", " - ", " – "} {
			if strings.Contains(line, sep) {
				parts = strings.Split(line, sep)
				break
			}
		}
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			skipped = append(skipped, line)
			continue
		}
		c := &Card{Deck: deck, Front: parts[0], Back: parts[1]}
		if len(parts) > 2 {
			c.Example = parts[2]
		}
		if len(parts) > 3 {
			c.Notes = strings.Join(parts[3:], "; ")
		}
		cards = append(cards, c)
	}
	return cards, skipped
}

func (a *App) importCards(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{"Decks": a.store.Decks()}
	if r.Method == http.MethodPost {
		deck := strings.TrimSpace(r.FormValue("deck"))
		text := r.FormValue("text")
		cards, skipped := parseImport(text, deck)
		if len(cards) > 0 {
			if err := a.store.AddMany(cards); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		data["Imported"] = len(cards)
		data["Skipped"] = skipped
		data["LastDeck"] = deck
		if len(cards) == 0 {
			data["Text"] = text
		}
		data["Decks"] = a.store.Decks()
	}
	a.render(w, "import", data)
}

func (a *App) cards(w http.ResponseWriter, r *http.Request) {
	deck := r.URL.Query().Get("deck")
	q := r.URL.Query().Get("q")
	a.render(w, "cards", map[string]any{
		"Cards": a.store.List(deck, q), "Decks": a.store.Decks(), "Deck": deck, "Q": q,
	})
}

func (a *App) edit(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.FormValue("id"))
	card := a.store.Get(id)
	if card == nil {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodPost {
		f := cardFromForm(r)
		if f.Front == "" || f.Back == "" {
			a.render(w, "edit", map[string]any{"Card": card, "Decks": a.store.Decks(), "Error": "German word and translation are both required."})
			return
		}
		if f.Deck == "" {
			f.Deck = "Default"
		}
		err := a.store.Update(id, func(c *Card) {
			c.Deck, c.Front, c.Back, c.Example, c.Notes = f.Deck, f.Front, f.Back, f.Example, f.Notes
			if r.FormValue("reset") == "on" {
				c.Reps, c.Lapses, c.Interval, c.Ease, c.Due = 0, 0, 0, 2.5, time.Now()
			}
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/cards?deck="+url.QueryEscape(f.Deck), http.StatusSeeOther)
		return
	}
	a.render(w, "edit", map[string]any{"Card": card, "Decks": a.store.Decks()})
}

func (a *App) deleteCard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/cards", http.StatusSeeOther)
		return
	}
	id, _ := strconv.Atoi(r.FormValue("id"))
	if err := a.store.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	http.Redirect(w, r, "/cards", http.StatusSeeOther)
}

func (a *App) export(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="german-cards.json"`)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(a.store.List("", ""))
}
