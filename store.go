package main

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// Card is a single flash card. Front is German, Back is the translation.
type Card struct {
	ID       int       `json:"id"`
	Deck     string    `json:"deck"`
	Front    string    `json:"front"`
	Back     string    `json:"back"`
	Example  string    `json:"example,omitempty"`
	Notes    string    `json:"notes,omitempty"`
	Created  time.Time `json:"created"`
	Due      time.Time `json:"due"`
	Interval float64   `json:"interval"` // days
	Ease     float64   `json:"ease"`
	Reps     int       `json:"reps"`
	Lapses   int       `json:"lapses"`
}

func (c *Card) IsNew() bool { return c.Reps == 0 }

type Store struct {
	mu     sync.Mutex
	path   string
	NextID int     `json:"next_id"`
	Cards  []*Card `json:"cards"`
}

func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, NextID: 1}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, err
	}
	return s, nil
}

// save writes atomically; caller must hold the lock.
func (s *Store) save() error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) Add(c *Card) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addLocked(c)
}

func (s *Store) AddMany(cards []*Card) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range cards {
		s.prepare(c)
		s.Cards = append(s.Cards, c)
	}
	return s.save()
}

func (s *Store) addLocked(c *Card) error {
	s.prepare(c)
	s.Cards = append(s.Cards, c)
	return s.save()
}

func (s *Store) prepare(c *Card) {
	now := time.Now()
	c.ID = s.NextID
	s.NextID++
	c.Created = now
	c.Due = now
	c.Ease = 2.5
	if strings.TrimSpace(c.Deck) == "" {
		c.Deck = "Default"
	}
}

func (s *Store) Get(id int) *Card {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.Cards {
		if c.ID == id {
			cp := *c
			return &cp
		}
	}
	return nil
}

// Update replaces the editable fields of a card (or its scheduling via fn).
func (s *Store) Update(id int, fn func(c *Card)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.Cards {
		if c.ID == id {
			fn(c)
			return s.save()
		}
	}
	return errors.New("card not found")
}

func (s *Store) Delete(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, c := range s.Cards {
		if c.ID == id {
			s.Cards = append(s.Cards[:i], s.Cards[i+1:]...)
			return s.save()
		}
	}
	return errors.New("card not found")
}

// List returns copies of cards matching the deck (empty = all) and query.
func (s *Store) List(deck, query string) []Card {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := strings.ToLower(strings.TrimSpace(query))
	var out []Card
	for _, c := range s.Cards {
		if deck != "" && c.Deck != deck {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(c.Front+" "+c.Back+" "+c.Example+" "+c.Notes), q) {
			continue
		}
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

func (s *Store) Decks() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[string]bool{}
	var decks []string
	for _, c := range s.Cards {
		if !seen[c.Deck] {
			seen[c.Deck] = true
			decks = append(decks, c.Deck)
		}
	}
	sort.Strings(decks)
	return decks
}

type DeckStats struct {
	Name     string
	Total    int
	New      int
	Due      int
	Learning int
}

func (s *Store) Stats() []DeckStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	m := map[string]*DeckStats{}
	for _, c := range s.Cards {
		d := m[c.Deck]
		if d == nil {
			d = &DeckStats{Name: c.Deck}
			m[c.Deck] = d
		}
		d.Total++
		switch {
		case c.IsNew():
			d.New++
		case !c.Due.After(now):
			d.Due++
		}
		if !c.IsNew() && c.Interval < 1 {
			d.Learning++
		}
	}
	var out []DeckStats
	for _, d := range m {
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// NextDue picks the next card to study: due reviews first (most overdue),
// then new cards (oldest first) when allowNew is set.
func (s *Store) NextDue(deck string, allowNew bool) *Card {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	var best, bestNew *Card
	for _, c := range s.Cards {
		if deck != "" && c.Deck != deck {
			continue
		}
		if c.IsNew() {
			if allowNew && (bestNew == nil || c.ID < bestNew.ID) {
				bestNew = c
			}
			continue
		}
		if !c.Due.After(now) && (best == nil || c.Due.Before(best.Due)) {
			best = c
		}
	}
	if best == nil {
		best = bestNew
	}
	if best == nil {
		return nil
	}
	cp := *best
	return &cp
}
