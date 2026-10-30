package main

import (
	"context"
	"log"
	"strings"
	"time"
	"unicode/utf8"
)

const maxRoomDescRunes = 200

// cleanRoomDescription tidies a room description: no line breaks or control characters, spaces squeezed,
// at most maxRoomDescRunes characters. ok is false when it is too long, so nothing is silently cut.
func cleanRoomDescription(raw string) (string, bool) {
	s := strings.Join(strings.Fields(clean(raw)), " ")
	if utf8.RuneCountInString(s) > maxRoomDescRunes {
		return "", false
	}
	return s, true
}

// markJoined records a membership the first time it is needed in this run. It never charges.
func (h *hub) markJoined(ctx context.Context, ou *onlineUser, slug string) {
	h.mu.Lock()
	_, already := ou.joined[slug]
	h.mu.Unlock()
	if already {
		return
	}
	if err := h.st.joinMember(ctx, slug, ou.id); err != nil {
		log.Printf("recording a room join failed: %v", err)
		return
	}
	h.mu.Lock()
	ou.joined[slug] = struct{}{} // joined, but a free public room still shows on the hub only while people are in it
	h.mu.Unlock()
	h.pushRooms(ou)
}

// roomLeave handles {"t":"rleave","r":"slug"}: stop being a member of a community room.
// The right to come back without paying again is kept. The host can not leave, they close the room instead.
func (h *hub) roomLeave(ctx context.Context, cl *client, in inbound) {
	ou := cl.user
	h.mu.Lock()
	rm := h.rooms[in.R]
	owner := rm != nil && rm.ownerID == ou.id
	_, joined := ou.joined[in.R]
	h.mu.Unlock()
	switch {
	case rm == nil || !rm.community():
		cl.enqueue(errFrame("no_such_room"))
		return
	case owner:
		cl.enqueue(errFrame("owner_cannot_leave"))
		return
	case !joined:
		cl.enqueue(errFrame("not_member"))
		return
	}
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	if _, err := h.st.leaveMember(dctx, rm.slug, ou.id); err != nil {
		log.Printf("leaving a room failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	h.mu.Lock()
	delete(ou.joined, rm.slug)
	for c := range ou.clients {
		if c.room == rm {
			delete(rm.clients, c)
			c.room = nil
			c.enqueue(frame(map[string]string{"t": "left"}))
		}
		c.enqueue(h.roomsFrameForLocked(ou))
	}
	h.departedLocked(rm, ou, true)
	h.mu.Unlock()
	cl.enqueue(frame(map[string]string{"t": "rleft", "s": rm.slug, "n": rm.name}))
}

// roomManage handles {"t":"rmgr","r":"slug"}: everything the room management page shows.
// Only the host (or staff) gets an answer. Reporter names are never included.
func (h *hub) roomManage(ctx context.Context, cl *client, in inbound) {
	h.mu.Lock()
	rm := h.hostRoomForLocked(cl, in.R)
	h.mu.Unlock()
	if rm == nil {
		cl.enqueue(errFrame("not_host"))
		return
	}
	dctx, cancel := context.WithTimeout(ctx, 2*authWait)
	defer cancel()
	members, err := h.st.roomMembers(dctx, rm.slug, 200)
	var reps []roomReportRow
	if err == nil {
		reps, err = h.st.roomReports(dctx, rm.slug, 50)
	}
	if err != nil {
		log.Printf("loading the room page failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	h.mu.Lock()
	now := time.Now()
	ms := make([]map[string]any, 0, len(members))
	for _, m := range members {
		e := map[string]any{"h": m.Handle, "j": m.Joined, "l": m.Left}
		if m.Avatar != "" {
			e["v"] = m.Avatar
		}
		_, key, _ := normHandle(m.Handle)
		for id, ou := range h.users {
			if strings.EqualFold(ou.handle, m.Handle) {
				if until, ok := rm.muted[id]; ok && until.After(now) {
					e["mu"] = minutesLeft(until)
				}
				if until, ok := rm.banned[id]; ok && until.After(now) {
					e["ki"] = minutesLeft(until)
				}
				break
			}
		}
		_ = key
		ms = append(ms, e)
	}
	rs := make([]map[string]any, 0, len(reps))
	for _, r := range reps {
		rs = append(rs, map[string]any{"c": r.Reason, "d": r.Details, "ts": r.TS, "s": r.Status})
	}
	out := map[string]any{"t": "rmgrinfo", "s": rm.slug, "n": rm.name, "d": rm.description, "k": rm.kind, "f": rm.fee,
		"w": rm.passHash != "", "i": rm.invite, "u": h.roomUsers(rm), "m": ms, "r": rs}
	h.mu.Unlock()
	cl.enqueue(frame(out))
}

// leaveOtherJoined ends the Joined state of every other community room the person is in, once they have entered
// slug (step 19: one room at a time). The right to come back without paying again is kept, exactly like the Leave
// room button. Rooms the person hosts are never left this way.
func (h *hub) leaveOtherJoined(ctx context.Context, ou *onlineUser, slug string) {
	h.mu.Lock()
	var others []string
	for s := range ou.joined {
		if rm := h.rooms[s]; s != slug && rm != nil && rm.ownerID != ou.id {
			others = append(others, s)
		}
	}
	h.mu.Unlock()
	if len(others) == 0 {
		return
	}
	for _, s := range others {
		if _, err := h.st.leaveMember(ctx, s, ou.id); err != nil {
			log.Printf("leaving the previous room failed: %v", err)
			return
		}
		h.mu.Lock()
		delete(ou.joined, s)
		if old := h.rooms[s]; old != nil {
			h.memberRemovedLocked(old, ou.handle)
		}
		h.mu.Unlock()
	}
	h.pushRooms(ou)
}

// settle is how enter finishes: join the room, then drop the Joined state of the previous one.
func (h *hub) settle(ctx context.Context, cl *client, slug string, last int64) {
	if h.joinRoom(cl, slug, last) {
		h.noteVisit(cl.user, slug) // only after the entry went through every check
		h.leaveOtherJoined(ctx, cl.user, slug)
		h.sendEngagement(ctx, cl, last)
	}
}
