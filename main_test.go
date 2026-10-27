package main

import "testing"

func TestParseImport(t *testing.T) {
	text := `# comment
der Hund ; the dog ; Der Hund bellt.
die Katze|the cat
laufen = to run
Wie geht es dir? - How are you?
brot	bread
nur ein Wort
`
	cards, skipped := parseImport(text, "A1")
	if len(cards) != 5 {
		t.Fatalf("got %d cards, want 5", len(cards))
	}
	if len(skipped) != 1 || skipped[0] != "nur ein Wort" {
		t.Errorf("skipped = %v", skipped)
	}
	if c := cards[0]; c.Front != "der Hund" || c.Back != "the dog" || c.Example != "Der Hund bellt." || c.Deck != "A1" {
		t.Errorf("card 0 = %+v", c)
	}
	if c := cards[3]; c.Front != "Wie geht es dir?" || c.Back != "How are you?" {
		t.Errorf("card 3 = %+v", c)
	}
}

func TestScheduleProgression(t *testing.T) {
	c := Card{Ease: 2.5}
	applyReview(&c, Good) // new -> 10 min step
	if c.Interval >= 1 {
		t.Fatalf("expected learning step, got %v days", c.Interval)
	}
	applyReview(&c, Good) // graduate -> 1 day
	if c.Interval != 1 {
		t.Fatalf("expected 1 day, got %v", c.Interval)
	}
	applyReview(&c, Good) // 1 * 2.5
	if c.Interval != 2.5 {
		t.Fatalf("expected 2.5 days, got %v", c.Interval)
	}
	applyReview(&c, Again)
	if c.Interval >= 1 || c.Lapses != 1 || c.Ease >= 2.5 {
		t.Fatalf("lapse not applied: %+v", c)
	}
}
