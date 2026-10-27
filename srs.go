package main

import (
	"fmt"
	"math"
	"time"
)

// Rating values, as in Anki.
const (
	Again = 1
	Hard  = 2
	Good  = 3
	Easy  = 4
)

// learning steps (in days) for new / lapsed cards: 1 min, 10 min
var learnSteps = []float64{1.0 / 1440, 10.0 / 1440}

// schedule applies a simplified SM-2 algorithm and returns the new
// interval (days) and ease. It does not mutate the card.
func schedule(c Card, rating int) (interval, ease float64) {
	ease = c.Ease
	if ease == 0 {
		ease = 2.5
	}
	iv := c.Interval

	// Still in learning phase (new card or interval below one day).
	if iv < 1 {
		switch rating {
		case Again:
			return learnSteps[0], ease
		case Hard:
			if iv < learnSteps[1] {
				return learnSteps[1] / 2, ease
			}
			return learnSteps[1], ease
		case Good:
			if iv < learnSteps[1] {
				return learnSteps[1], ease
			}
			return 1, ease // graduate
		case Easy:
			return 4, ease
		}
	}

	switch rating {
	case Again:
		ease = math.Max(1.3, ease-0.2)
		return learnSteps[1], ease // relearn
	case Hard:
		ease = math.Max(1.3, ease-0.15)
		return math.Max(iv+1, iv*1.2), ease
	case Good:
		return math.Max(iv+1, iv*ease), ease
	case Easy:
		ease += 0.15
		return math.Max(iv+1, iv*ease*1.3), ease
	}
	return iv, ease
}

func applyReview(c *Card, rating int) {
	iv, ease := schedule(*c, rating)
	if rating == Again && c.Interval >= 1 {
		c.Lapses++
	}
	c.Interval = iv
	c.Ease = ease
	c.Reps++
	c.Due = time.Now().Add(time.Duration(iv * 24 * float64(time.Hour)))
}

// humanInterval formats a duration in days like "10m", "1d", "3mo".
func humanInterval(days float64) string {
	mins := days * 1440
	switch {
	case mins < 60:
		return fmt.Sprintf("%.0fm", math.Max(1, math.Round(mins)))
	case days < 1:
		return fmt.Sprintf("%.0fh", math.Round(mins/60))
	case days < 30:
		return fmt.Sprintf("%.0fd", math.Round(days))
	case days < 365:
		return fmt.Sprintf("%.1fmo", days/30)
	default:
		return fmt.Sprintf("%.1fy", days/365)
	}
}
