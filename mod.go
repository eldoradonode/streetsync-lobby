package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultSanctionMinutes = 10
	maxSanctionMinutes     = 1440 // one day
	maxReportsPerHour      = 10
	listReportsLimit       = 50
	kickFlushDelay         = 300 * time.Millisecond // lets the "you were removed" frame go out first
)

func roleRank(role string) int {
	switch role {
	case "admin":
		return 3
	case "mod":
		return 2
	}
	return 1
}

// canAct says whether someone with the actor role may act on someone with the target role.
// Moderators handle normal users. Admins handle moderators and normal users. Nobody acts on an admin.
func canAct(actor, target string) bool {
	return roleRank(actor) >= 2 && roleRank(actor) > roleRank(target)
}

func minutesLeft(until time.Time) int {
	d := time.Until(until)
	if d <= 0 {
		return 0
	}
	return int(math.Ceil(d.Minutes()))
}

func (h *hub) mutedMinutes(ou *onlineUser) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return minutesLeft(ou.mutedUntil)
}

// broadcastLocked sends to everyone in the room. The caller holds the lock.
func (h *hub) broadcastLocked(b []byte) {
	for cl := range h.clients {
		cl.enqueue(b)
	}
}

// withoutDeleted drops messages a moderator removed before they were saved.
func (h *hub) withoutDeleted(batch []storedMsg) []storedMsg {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.deleted) == 0 && len(h.gone) == 0 {
		return batch
	}
	out := batch[:0]
	for _, m := range batch {
		if _, removed := h.deleted[m.ID]; removed {
			continue
		}
		if _, dead := h.gone[m.UserID]; dead {
			continue
		}
		out = append(out, m)
	}
	return out
}

// findMsgLocked looks for a message in every room. Ids are unique across rooms.
func (h *hub) findMsgLocked(id int64) (*room, *chatMsg) {
	for _, rm := range h.roomList {
		if m := findMsg(rm, id); m != nil {
			return rm, m
		}
	}
	return nil, nil
}

// broadcastAround sends to every room the person is in, so the people who can see them are told.
// Caller holds the lock.
func (h *hub) broadcastAround(ou *onlineUser, b []byte) {
	if ou == nil {
		return
	}
	seen := map[*room]bool{}
	for c := range ou.clients {
		if c.room != nil && !seen[c.room] {
			seen[c.room] = true
			for rc := range c.room.clients {
				rc.enqueue(b)
			}
		}
	}
}

// ---- reports ----

type repJSON struct {
	K    string `json:"k,omitempty"`  // message (left out), user or room
	Room string `json:"rm,omitempty"` // room slug, for room reports
	Det  string `json:"d,omitempty"`  // what the reporter wrote
	ID   int64  `json:"id"`
	By   string `json:"by"`
	Who  string `json:"h"`
	Msg  int64  `json:"i"`
	Body string `json:"b"`
	Why  string `json:"c"`
	TS   int64  `json:"ts"`
}

// report lets any user report a message that is still in the room's recent history.
func (h *hub) report(ctx context.Context, cl *client, in inbound) {
	ou := cl.user
	if !validReportReason(in.C) {
		cl.enqueue(errFrame("bad_report"))
		return
	}
	now := time.Now()
	h.mu.Lock()
	keep := ou.reportTimes[:0]
	for _, t := range ou.reportTimes {
		if t.After(now.Add(-time.Hour)) {
			keep = append(keep, t)
		}
	}
	ou.reportTimes = keep
	tooMany := len(keep) >= maxReportsPerHour
	// Only a message in the room you are standing in can be reported, so message ids can not be used to probe other rooms.
	var m *chatMsg
	if cl.room != nil {
		m = findMsg(cl.room, in.I)
	}
	h.mu.Unlock()

	switch {
	case tooMany:
		cl.enqueue(errFrame("slow_down"))
		return
	case m == nil:
		cl.enqueue(errFrame("report_gone"))
		return
	case strings.EqualFold(m.Handle, ou.handle):
		cl.enqueue(errFrame("self_report"))
		return
	}

	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	// Staff are not reported through the ordinary queue, which moderators read. They have their own channel (scomp).
	_, akey, _ := normHandle(m.Handle)
	if au, aerr := h.st.userByKey(dctx, akey); aerr == nil && isStaff(au.Role) {
		cl.enqueue(errFrame("staff_report"))
		return
	} else if aerr != nil && !errors.Is(aerr, errNotFound) {
		cl.enqueue(errFrame("server_busy"))
		return
	}
	_, dup, err := h.st.addReport(dctx, reportRow{
		ReporterID: ou.id, ReporterHandle: ou.handle, MsgID: m.ID,
		TargetHandle: m.Handle, Body: m.Body, Reason: in.C,
	})
	if err != nil {
		log.Printf("saving report failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	if dup {
		cl.enqueue(errFrame("already_reported"))
		return
	}
	h.mu.Lock()
	ou.reportTimes = append(ou.reportTimes, now)
	h.mu.Unlock()
	cl.enqueue(frame(map[string]string{"t": "repok"}))
	h.sendReportCount(dctx)
}

func (h *hub) listReports(ctx context.Context, cl *client) {
	h.mu.Lock()
	allowed := roleRank(cl.user.role) >= 2
	h.mu.Unlock()
	if !allowed {
		cl.enqueue(errFrame("forbidden"))
		return
	}
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	rows, err := h.st.openReportsFor(dctx, listReportsLimit, h.isAdmin(cl.user))
	if err != nil {
		log.Printf("listing reports failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	out := make([]repJSON, 0, len(rows))
	for _, r := range rows {
		k := r.Kind
		if k == "message" {
			k = ""
		}
		out = append(out, repJSON{K: k, Room: r.TargetRoom, Det: r.Details, ID: r.ID, By: r.ReporterHandle, Who: r.TargetHandle, Msg: r.MsgID, Body: r.Body, Why: r.Reason, TS: r.TS})
	}
	cl.enqueue(frame(map[string]any{"t": "reps", "r": out}))
}

// sendReportCount tells every moderator who is online how many reports are open.
func (h *hub) sendReportCount(ctx context.Context) {
	all, err := h.st.openReportCountFor(ctx, true)
	if err != nil {
		return
	}
	modOnly, err := h.st.openReportCountFor(ctx, false)
	if err != nil {
		return
	}
	ball := frame(map[string]any{"t": "rn", "c": all})
	bmod := frame(map[string]any{"t": "rn", "c": modOnly})
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, ou := range h.users {
		if roleRank(ou.role) >= 2 {
			b := bmod
			if ou.role == "admin" {
				b = ball
			}
			for c := range ou.clients {
				c.enqueue(b)
			}
		}
	}
}

func (h *hub) sendReportCountTo(cl *client) {
	ctx, cancel := context.WithTimeout(context.Background(), authWait)
	defer cancel()
	h.mu.Lock()
	admin := cl.user.role == "admin"
	h.mu.Unlock()
	if n, err := h.st.openReportCountFor(ctx, admin); err == nil {
		cl.enqueue(frame(map[string]any{"t": "rn", "c": n}))
	}
	if admin {
		if n, err := h.st.openComplaintCount(ctx, cl.user.id); err == nil {
			cl.enqueue(frame(map[string]any{"t": "cn", "c": n}))
		}
	}
}

// ---- moderator actions ----

func (h *hub) modAction(ctx context.Context, cl *client, in inbound) {
	actor := cl.user
	h.mu.Lock()
	role := actor.role
	h.mu.Unlock()
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	if in.A == "del" {
		h.deleteMsg(dctx, cl, role, in) // members may delete their own, moderators may delete more
		return
	}
	if roleRank(role) < 2 {
		cl.enqueue(errFrame("forbidden"))
		return
	}

	switch in.A {
	case "grant":
		h.adminGrant(dctx, cl, in)
	case "dismiss":
		ok, err := h.st.closeReport(dctx, in.ID, "dismissed", actor.handle)
		if err != nil {
			log.Printf("closing report failed: %v", err)
			cl.enqueue(errFrame("server_busy"))
			return
		}
		if !ok {
			cl.enqueue(errFrame("report_gone"))
			return
		}
		h.st.logMod(dctx, actor.handle, "dismiss", "", fmt.Sprintf("report %d", in.ID))
		cl.enqueue(frame(map[string]any{"t": "modok", "a": "dismiss", "id": in.ID}))
		h.sendReportCount(dctx)
	case "mute", "kick", "unmute", "promote", "demote", "clearbio", "ban", "unban":
		h.userAction(dctx, cl, role, in)
	default:
		cl.enqueue(errFrame("bad_action"))
	}
}

func (h *hub) userAction(ctx context.Context, cl *client, role string, in inbound) {
	actor := cl.user
	_, key, ok := normHandle(in.H)
	if !ok {
		cl.enqueue(errFrame("no_such_user"))
		return
	}
	target, err := h.st.userByKey(ctx, key)
	if errors.Is(err, errNotFound) {
		cl.enqueue(errFrame("no_such_user"))
		return
	}
	if err != nil {
		log.Printf("moderation lookup failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	if target.ID == actor.id || !canAct(role, target.Role) {
		cl.enqueue(errFrame("forbidden"))
		return
	}
	if (in.A == "promote" || in.A == "demote" || in.A == "ban" || in.A == "unban") && role != "admin" {
		cl.enqueue(errFrame("forbidden"))
		return
	}

	mins := in.M
	if mins <= 0 {
		mins = defaultSanctionMinutes
	}
	if mins > maxSanctionMinutes {
		mins = maxSanctionMinutes
	}
	fail := func(err error) {
		log.Printf("moderation %s failed: %v", in.A, err)
		cl.enqueue(errFrame("server_busy"))
	}
	detail := ""

	switch in.A {
	case "mute":
		until, err := h.st.addSanction(ctx, target.ID, "mute", actor.handle, "", mins)
		if err != nil {
			fail(err)
			return
		}
		h.mu.Lock()
		ou := h.users[target.ID]
		if ou != nil {
			ou.mutedUntil = until
			f := frame(map[string]any{"t": "muted", "m": mins})
			for c := range ou.clients {
				c.enqueue(f)
			}
		}
		h.broadcastAround(ou, frame(map[string]any{"t": "mod", "a": "mute", "h": target.Handle, "m": mins}))
		h.mu.Unlock()
		detail = fmt.Sprintf("%d minutes", mins)

	case "kick":
		if _, err := h.st.addSanction(ctx, target.ID, "kick", actor.handle, "", mins); err != nil {
			fail(err)
			return
		}
		var victims []*client
		h.mu.Lock()
		ou := h.users[target.ID]
		if ou != nil {
			f := frame(map[string]any{"t": "kicked", "m": mins})
			for c := range ou.clients {
				c.enqueue(f)
				victims = append(victims, c)
			}
		}
		h.broadcastAround(ou, frame(map[string]any{"t": "mod", "a": "kick", "h": target.Handle, "m": mins}))
		h.mu.Unlock()
		for _, c := range victims {
			time.AfterFunc(kickFlushDelay, c.cancel)
		}
		detail = fmt.Sprintf("%d minutes", mins)

	case "unmute":
		had, err := h.st.clearMute(ctx, target.ID)
		if err != nil {
			fail(err)
			return
		}
		if !had {
			cl.enqueue(errFrame("not_muted"))
			return
		}
		h.mu.Lock()
		if ou := h.users[target.ID]; ou != nil {
			ou.mutedUntil = time.Time{}
			f := frame(map[string]string{"t": "unmuted"})
			for c := range ou.clients {
				c.enqueue(f)
			}
		}
		h.mu.Unlock()

	case "clearbio":
		if err := h.st.setBio(ctx, target.ID, ""); err != nil {
			fail(err)
			return
		}
		h.mu.Lock()
		if ou := h.users[target.ID]; ou != nil {
			ou.bio = ""
			f := frame(meFrame{T: "me", meFields: ou.me()})
			for c := range ou.clients {
				c.enqueue(f)
			}
		}
		h.mu.Unlock()
		detail = "bio cleared"

	case "ban":
		reason := strings.TrimSpace(clean(in.B))
		if utf8.RuneCountInString(reason) > 200 {
			reason = string([]rune(reason)[:200])
		}
		if err := h.st.banUser(ctx, target.ID, actor.handle, reason); err != nil {
			switch {
			case errors.Is(err, errAlreadyBanned):
				cl.enqueue(errFrame("already_banned"))
			case errors.Is(err, errAdminTarget), errors.Is(err, errNotFound):
				cl.enqueue(errFrame("forbidden"))
			default:
				fail(err)
			}
			return
		}
		h.cutOff(target.ID, "banned")
		cl.enqueue(frame(map[string]any{"t": "modok", "a": "ban", "h": target.Handle}))
		return // the audit entry was written in the same transaction as the ban
	case "unban":
		if err := h.st.unbanUser(ctx, target.ID, actor.handle); err != nil {
			if errors.Is(err, errNotBanned) {
				cl.enqueue(errFrame("not_banned"))
			} else {
				fail(err)
			}
			return
		}
		cl.enqueue(frame(map[string]any{"t": "modok", "a": "unban", "h": target.Handle}))
		return

	case "promote", "demote":
		newRole := "mod"
		if in.A == "demote" {
			newRole = "user"
			if target.Role != "mod" {
				cl.enqueue(errFrame("forbidden"))
				return
			}
		} else if !target.PassHash.Valid {
			cl.enqueue(errFrame("needs_password"))
			return
		}
		if err := h.st.setRole(ctx, target.ID, newRole); err != nil {
			fail(err)
			return
		}
		h.mu.Lock()
		if isStaff(newRole) {
			for _, rm := range h.roomList {
				if rm.ownerID == target.ID {
					rm.fee = 0 // the database was updated in setRole
				}
			}
		}
		if ou := h.users[target.ID]; ou != nil {
			ou.role = newRole
			f := frame(meFrame{T: "me", meFields: ou.me()})
			for c := range ou.clients {
				c.enqueue(f)
			}
		}
		h.mu.Unlock()
		detail = newRole
	}

	h.st.logMod(ctx, actor.handle, in.A, target.Handle, detail)
	if in.RID > 0 && (in.A == "mute" || in.A == "kick") {
		if _, err := h.st.closeReport(ctx, in.RID, "actioned:"+in.A, actor.handle); err == nil {
			h.sendReportCount(ctx)
		}
	}
	cl.enqueue(frame(map[string]any{"t": "modok", "a": in.A, "h": target.Handle}))
}

// deleteMsg removes a message for everyone in its room.
// Anyone may remove their own message. Moderators may also remove messages of people they could mute.
func (h *hub) deleteMsg(ctx context.Context, cl *client, role string, in inbound) {
	actor := cl.user
	h.mu.Lock()
	rm, m := h.findMsgLocked(in.I)
	h.mu.Unlock()
	if m == nil {
		// Not a room message. A moderator may be removing a reported direct message.
		if roleRank(role) >= 2 && h.deleteReportedDM(ctx, cl, in) {
			return
		}
		cl.enqueue(errFrame("msg_gone"))
		return
	}
	if m.TipAmt > 0 {
		cl.enqueue(errFrame("forbidden")) // a tip card is the record of a payment: nobody removes it
		return
	}
	mine := strings.EqualFold(m.Handle, actor.handle)
	hostOnly := false
	if !mine && roleRank(role) < 2 {
		// The host of a community room can remove what ordinary members say in it, never what staff say.
		h.mu.Lock()
		hostOnly = rm.community() && rm.ownerID == actor.id && m.Role == ""
		h.mu.Unlock()
		if hostOnly {
			// The message may be older than the author's promotion: read the role now, from the database.
			_, akey, _ := normHandle(m.Handle)
			if au, aerr := h.st.userByKey(ctx, akey); aerr == nil && isStaff(au.Role) {
				hostOnly = false
			} else if aerr != nil && !errors.Is(aerr, errNotFound) {
				cl.enqueue(errFrame("server_busy"))
				return
			}
		}
		if !hostOnly {
			cl.enqueue(errFrame("forbidden"))
			return
		}
	}
	if !mine && !hostOnly {
		if roleRank(role) < 2 {
			cl.enqueue(errFrame("forbidden"))
			return
		}
		// If the author's account no longer exists, any moderator may remove the message.
		_, key, _ := normHandle(m.Handle)
		author, err := h.st.userByKey(ctx, key)
		if err == nil && !canAct(role, author.Role) {
			cl.enqueue(errFrame("forbidden"))
			return
		}
		if err != nil && !errors.Is(err, errNotFound) {
			log.Printf("moderation lookup failed: %v", err)
			cl.enqueue(errFrame("server_busy"))
			return
		}
	}
	if err := h.st.deleteMessage(ctx, m.ID); err != nil {
		log.Printf("deleting message failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	h.mu.Lock()
	for i := range rm.buf {
		if rm.buf[i].ID == m.ID {
			rm.buf = append(rm.buf[:i], rm.buf[i+1:]...)
			break
		}
	}
	h.deleted[m.ID] = struct{}{}
	h.unpinDeletedLocked(rm, m.ID)
	del := frame(map[string]any{"t": "del", "i": m.ID})
	for c := range rm.clients {
		c.enqueue(del)
	}
	h.mu.Unlock()

	if !mine && !hostOnly {
		h.st.logMod(ctx, actor.handle, "del", m.Handle, snippet(m.Body))
		if in.RID > 0 {
			if _, err := h.st.closeReport(ctx, in.RID, "actioned:del", actor.handle); err == nil {
				h.sendReportCount(ctx)
			}
		}
	}
	cl.enqueue(frame(map[string]any{"t": "modok", "a": "del", "i": m.ID}))
}

// userDelete is the frame members use to delete their own message.
func (h *hub) userDelete(ctx context.Context, cl *client, in inbound) {
	h.mu.Lock()
	role := cl.user.role
	h.mu.Unlock()
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	h.deleteMsg(dctx, cl, role, in)
}
