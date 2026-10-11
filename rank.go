package main

import (
	"strings"
	"time"
	"unicode/utf8"
)

// Tier names are Newbie, Regular, Insider, Elite, Legend (indexes 0 to 4).
// The web page holds the display names. The server only sends the index.
var tierStarts = []int64{0, 300, 2000, 10000, 40000}

// Point rules. These are starting values to tune after real users show up.
// Time spent connected earns nothing: points only come from real messages.
const (
	pointsPerMessage   = 1
	minScoringRunes    = 3                // shorter messages are sent but earn nothing
	hourlyPointCap     = 60               // message points per user per hour
	streakBonusUnit    = 10               // daily bonus = unit x streak days, capped below
	streakBonusMaxDays = 7                // bonus stops growing after this many days
	repeatWindow       = 30 * time.Second // the same text again within this window earns nothing
)

func tierFor(points int64) int {
	for i := len(tierStarts) - 1; i > 0; i-- {
		if points >= tierStarts[i] {
			return i
		}
	}
	return 0
}

// nextTierAt returns the points needed for the next tier, or 0 at the top tier.
func nextTierAt(tier int) int64 {
	if tier+1 >= len(tierStarts) {
		return 0
	}
	return tierStarts[tier+1]
}

// score applies the point rules to one accepted message and updates the user.
// The caller holds the hub lock.
func (u *onlineUser) score(body string, now time.Time) (gain, bonus int64) {
	repeat := strings.EqualFold(body, u.lastBody) && now.Sub(u.lastBodyAt) < repeatWindow
	u.lastBody, u.lastBodyAt = body, now
	if utf8.RuneCountInString(body) < minScoringRunes || repeat {
		return 0, 0
	}
	if now.Sub(u.hourStart) >= time.Hour {
		u.hourStart, u.hourPoints = now, 0
	}
	if u.hourPoints >= hourlyPointCap {
		return 0, 0
	}
	gain = pointsPerMessage
	u.hourPoints += gain

	// Daily streak: the first scoring message of each UTC day counts.
	day := int(now.UTC().Unix() / 86400)
	if u.streakDay != day {
		if u.streakDay == day-1 {
			u.streakDays++
		} else {
			u.streakDays = 1
		}
		u.streakDay = day
		bonus = int64(streakBonusUnit * min(u.streakDays, streakBonusMaxDays))
	}
	u.points += gain + bonus
	u.dirty = true
	return gain, bonus
}
