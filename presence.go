package main

import (
	"sync"
	"time"
)

// Room presence notices. A notice is a live "@bob joined" or "@bob left" line. It is never stored in the room history,
// never counted as a message, and never earns points or credits, because those are only given in post().
//
// A person counts as here in ONE room at a time (see joinRoom). They count as here from the moment their first connection enters a room. They stay here through a dropped
// connection or a phone going to the background for graceLeave, so a short network gap makes no notice at all.
// Membership of a community room (room_members) is not touched by any of this: only the Leave room action ends it.
var graceLeave = 60 * time.Second

// graceMu lets tests shorten graceLeave without racing with connections that are still closing.
// backgroundGrace is how long a person whose page was in the background when the connection was lost stays in the
// room. enterSettle is the extra wait for a connection that is still being put back into the room.
var (
	backgroundGrace = 5 * time.Minute
	enterSettle     = 15 * time.Second
	graceMu         sync.RWMutex
)

func backgroundGraceDur() time.Duration {
	graceMu.RLock()
	defer graceMu.RUnlock()
	return backgroundGrace
}

func graceDur() time.Duration {
	graceMu.RLock()
	defer graceMu.RUnlock()
	return graceLeave
}

func (rm *room) hasUser(id int64) bool {
	for c := range rm.clients {
		if c.user != nil && c.user.id == id {
			return true
		}
	}
	return false
}

func (h *hub) presenceNoteLocked(rm *room, ou *onlineUser, joined bool) {
	a := "l"
	if joined {
		a = "j"
	}
	b := frame(map[string]string{"t": "rs", "a": a, "h": ou.handle})
	for c := range rm.clients {
		if c.user != ou {
			c.enqueue(b)
		}
	}
	// The member list changes exactly when the notice does, never earlier or later (step 20).
	if rm.hereWho == nil {
		rm.hereWho = map[int64]*onlineUser{}
	}
	if joined {
		rm.hereWho[ou.id] = ou
	} else {
		delete(rm.hereWho, ou.id)
	}
	h.memberPresenceLocked(rm, ou, joined)
}

// arrivedLocked is called after a client was added to rm.clients.
func (h *hub) arrivedLocked(rm *room, ou *onlineUser) {
	if t := rm.grace[ou.id]; t != nil {
		t.Stop()
		delete(rm.grace, ou.id) // came back inside the grace period: no second notice
	}
	if rm.here == nil {
		rm.here = map[int64]bool{}
	}
	if !rm.here[ou.id] {
		rm.here[ou.id] = true
		h.presenceNoteLocked(rm, ou, true)
	}
}

// departedLocked is called after a client was removed from rm.clients. With immediate the notice goes out now,
// otherwise only when the person has not come back after graceLeave.
func (h *hub) departedLocked(rm *room, ou *onlineUser, immediate bool) {
	h.departedForLocked(rm, ou, immediate, graceDur())
}

// departedConnLocked handles a connection that was lost. What kind of loss it was decides how long the person is
// kept in the room before anyone is told:
//   - the login was ended on purpose, or the account was banned or deleted: they are not coming back, so at once;
//   - the page had told us it was in the background: a phone that locks its screen kills the connection without the
//     person leaving, so they are kept for backgroundGrace;
//   - anything else, a network loss or a refresh: graceLeave.
func (h *hub) departedConnLocked(rm *room, cl *client) {
	ou := cl.user
	switch {
	case cl.ended || ou.cut.Load():
		h.departedForLocked(rm, ou, true, 0)
	case cl.hidden:
		h.departedForLocked(rm, ou, false, backgroundGraceDur())
	default:
		h.departedForLocked(rm, ou, false, graceDur())
	}
}

// enteringLocked: does the person have a connection that is on its way into this room (a page that just
// reconnected and is still being put back)?
func (h *hub) enteringLocked(ou *onlineUser, slug string) bool {
	for c := range ou.clients {
		if c.room == nil && c.entering == slug {
			return true
		}
	}
	return false
}

func (h *hub) departedForLocked(rm *room, ou *onlineUser, immediate bool, grace time.Duration) {
	if rm == nil || ou == nil || rm.hasUser(ou.id) || !rm.here[ou.id] {
		return
	}
	if t := rm.grace[ou.id]; t != nil {
		t.Stop()
		delete(rm.grace, ou.id)
	}
	if immediate {
		delete(rm.here, ou.id)
		h.presenceNoteLocked(rm, ou, false)
		return
	}
	h.armGraceLocked(rm, ou, grace, false)
}

// armGraceLocked starts the timer that announces a departure. When it runs out the announcement is made only after
// checking that the person really has no connection in the room and none on its way into it. A connection that is
// still being put back gets one more short wait (enterSettle), then the notice is decided on what is true then.
func (h *hub) armGraceLocked(rm *room, ou *onlineUser, d time.Duration, settled bool) {
	if rm.grace == nil {
		rm.grace = map[int64]*time.Timer{}
	}
	var t *time.Timer
	t = time.AfterFunc(d, func() {
		safeGo(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if rm.grace[ou.id] != t {
				return // replaced or cancelled
			}
			delete(rm.grace, ou.id)
			if rm.hasUser(ou.id) || !rm.here[ou.id] {
				return
			}
			if !settled && h.enteringLocked(ou, rm.slug) {
				h.armGraceLocked(rm, ou, enterSettle, true)
				return
			}
			delete(rm.here, ou.id)
			h.presenceNoteLocked(rm, ou, false)
		})
	})
	rm.grace[ou.id] = t
}

// setHidden records that the page went to the background (bg) or came back (fg).
func (h *hub) setHidden(cl *client, hidden bool) {
	h.mu.Lock()
	cl.hidden = hidden
	h.mu.Unlock()
}
