package main

import "time"

// Online presence for the conversation header. It lives only in memory: nothing is written to the database, it is
// never part of a message, and it is only sent to someone who has that person's conversation open and is not blocked.
//
// A person is online while at least one of their connections is alive. The server pings every connection
// (see ws.go), so a dead connection is dropped and counted out. When the last connection goes, "offline" is announced
// only after presenceGrace, so a phone that blinks in the background or a quick reconnect does not flap the dot.
var presenceGrace = 10 * time.Second

type presState struct {
	online bool        // at least one live connection, offline only announced after presenceGrace
	shown  bool        // what watchers were last told: online AND the person allows being seen online
	timer  *time.Timer // a pending "offline" announcement
}

func (h *hub) presLocked(id int64) *presState {
	if h.pres == nil {
		h.pres = map[int64]*presState{}
	}
	p := h.pres[id]
	if p == nil {
		p = &presState{}
		h.pres[id] = p
	}
	return p
}

// isOnlineLocked is what other people may see. The setting "show when I am online" (hide_online) is respected, and
// until a person's settings are known they count as hidden, so nothing is revealed by accident.
func (h *hub) isOnlineLocked(id int64) bool {
	if p := h.pres[id]; p != nil {
		return p.shown
	}
	return false
}

// presenceRecomputeLocked tells watchers when what they may see has changed.
func (h *hub) presenceRecomputeLocked(ou *onlineUser) {
	h.memberVisibilityLocked(ou) // the member lists follow the same setting
	p := h.presLocked(ou.id)
	eff := p.online && ou.prefsLoaded && !ou.prefs.HideOnline
	if eff != p.shown {
		p.shown = eff
		h.tellWatchersLocked(ou, eff)
	}
}

func presFrame(handle string, on bool) []byte {
	o := "0"
	if on {
		o = "1"
	}
	return frame(map[string]string{"t": "pres", "h": handle, "o": o})
}

func (h *hub) tellWatchersLocked(ou *onlineUser, on bool) {
	b := presFrame(ou.handle, on)
	for c := range h.watchers[ou.id] {
		c.enqueue(b)
	}
}

// presenceAttachedLocked runs when a connection of ou was added.
func (h *hub) presenceAttachedLocked(ou *onlineUser) {
	p := h.presLocked(ou.id)
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
	p.online = true
	h.presenceRecomputeLocked(ou)
}

// presenceDetachedLocked runs when the last connection of ou went away.
func (h *hub) presenceDetachedLocked(ou *onlineUser) {
	p := h.presLocked(ou.id)
	if p.timer != nil {
		p.timer.Stop()
	}
	var t *time.Timer
	t = time.AfterFunc(presenceGrace, func() {
		safeGo(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if p.timer != t {
				return // came back, or replaced
			}
			p.timer = nil
			if len(ou.clients) == 0 && p.online {
				p.online = false
				h.presenceRecomputeLocked(ou)
			}
			if !p.online && len(h.watchers[ou.id]) == 0 {
				delete(h.pres, ou.id) // nothing to remember about someone who is offline and unwatched
			}
		})
	})
	p.timer = t
}

func (h *hub) unwatchLocked(cl *client) {
	if cl.watch == 0 {
		return
	}
	if m := h.watchers[cl.watch]; m != nil {
		delete(m, cl)
		if len(m) == 0 {
			delete(h.watchers, cl.watch)
		}
	}
	cl.watch = 0
}

// watchLocked makes cl follow one person's online state and returns it. A connection follows one person at a time.
func (h *hub) watchLocked(cl *client, id int64) bool {
	h.unwatchLocked(cl)
	if h.watchers == nil {
		h.watchers = map[int64]map[*client]struct{}{}
	}
	if h.watchers[id] == nil {
		h.watchers[id] = map[*client]struct{}{}
	}
	h.watchers[id][cl] = struct{}{}
	cl.watch = id
	return h.isOnlineLocked(id)
}

// presenceOpen is called when a conversation with other is shown to cl. It answers with the current state and
// then keeps cl up to date. A blocked pair gets nothing, so blocking also hides when someone is around.
func (h *hub) presenceOpen(cl *client, otherID int64, otherHandle string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	on := h.watchLocked(cl, otherID)
	cl.enqueue(presFrame(otherHandle, on))
}

// presenceClose stops following when the conversation is closed.
func (h *hub) presenceClose(cl *client) {
	h.mu.Lock()
	h.unwatchLocked(cl)
	h.mu.Unlock()
}
