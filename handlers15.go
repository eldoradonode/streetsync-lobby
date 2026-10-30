package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"log"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxDetailRunes    = 300 // what a reporter may write
	searchMinRunes    = 2
	searchLimit       = 20
	searchCooldown    = 250 * time.Millisecond
	deleteTokenTTL    = 5 * time.Minute
	deleteConfirmWord = "DELETE"
)

func validReportReason(c string) bool {
	switch c {
	case "spam", "abuse", "harassment", "scam", "other":
		return true
	}
	return false
}

// staffReasons are the categories of a complaint about staff.
func validStaffReason(c string) bool {
	switch c {
	case "abuse", "harassment", "unfair", "other":
		return true
	}
	return false
}

func cleanDetails(raw string, max int) (string, bool) {
	s := strings.Join(strings.Fields(clean(raw)), " ")
	if utf8.RuneCountInString(s) > max {
		return "", false
	}
	return s, true
}

func (h *hub) isAdmin(ou *onlineUser) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return ou.role == "admin"
}

// reportBudget uses one of the person's reports for the hour. It is shared by every kind of report and complaint.
func (h *hub) reportBudget(ou *onlineUser, now time.Time) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	keep := ou.reportTimes[:0]
	for _, t := range ou.reportTimes {
		if t.After(now.Add(-time.Hour)) {
			keep = append(keep, t)
		}
	}
	ou.reportTimes = keep
	return len(keep) < maxReportsPerHour
}

func (h *hub) reportSpent(ou *onlineUser, now time.Time) {
	h.mu.Lock()
	ou.reportTimes = append(ou.reportTimes, now)
	h.mu.Unlock()
}

// cutOff closes every connection of a person who was banned, and refuses anything they still have queued.
func (h *hub) cutOff(id int64, code string) {
	var victims []*client
	h.mu.Lock()
	if ou := h.users[id]; ou != nil {
		ou.cut.Store(true)
		f := errFrame(code)
		for c := range ou.clients {
			c.enqueue(f)
			victims = append(victims, c)
		}
	}
	h.mu.Unlock()
	for _, c := range victims {
		time.AfterFunc(kickFlushDelay, c.cancel)
	}
}

// purgeDeleted removes a deleted account from memory: connections closed, messages gone from every room buffer,
// rooms they owned dropped, and the members of those rooms told.
func (h *hub) purgeDeleted(id int64, res deleteResult) {
	var victims []*client
	h.mu.Lock()
	h.gone[id] = struct{}{}
	if ou := h.users[id]; ou != nil {
		ou.cut.Store(true)
		ou.dirty = false
		f := frame(map[string]string{"t": "deleted"})
		for c := range ou.clients {
			c.enqueue(f)
			victims = append(victims, c)
		}
	}
	for _, rm := range h.roomList {
		kept := rm.buf[:0]
		for _, m := range rm.buf {
			if strings.EqualFold(m.Handle, res.Handle) && m.Handle != "" {
				b := frame(map[string]any{"t": "del", "i": m.ID})
				for c := range rm.clients {
					c.enqueue(b)
				}
				h.unpinDeletedLocked(rm, m.ID)
				continue
			}
			kept = append(kept, m)
		}
		rm.buf = kept
		if p := rm.poll; p != nil { // their votes are deleted with the account (store15), so the live counts follow
			if opt, had := p.Votes[id]; had {
				if opt < len(p.Counts) {
					p.Counts[opt]--
				}
				delete(p.Votes, id)
				for c := range rm.clients {
					c.enqueue(frame(map[string]any{"t": "poll", "p": p.json(c.user.id)}))
				}
			}
		}
	}
	for _, cr := range res.ClosedRooms {
		if rm := h.rooms[cr.Slug]; rm != nil {
			h.dropRoomLocked(rm, "rclosed")
		}
		for _, mid := range cr.Members {
			if ou := h.users[mid]; ou != nil {
				delete(ou.rooms, cr.Slug)
				for c := range ou.clients {
					c.enqueue(frame(map[string]string{"t": "rclosed", "s": cr.Slug, "n": cr.Name}))
				}
			}
		}
	}
	delete(h.users, id)
	h.mu.Unlock()
	for _, c := range victims {
		time.AfterFunc(kickFlushDelay, c.cancel)
	}
}

// ---- people search ----

// userSearch handles {"t":"usearch","n":"ada"}. Any signed in person may search. Only public fields come back.
func (h *hub) userSearch(ctx context.Context, cl *client, in inbound) {
	ou := cl.user
	q := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(clean(in.N)), "@"))
	if n := utf8.RuneCountInString(q); n < searchMinRunes || n > 16 || !onlyHandleRunes(q) {
		cl.enqueue(errFrame("bad_query"))
		return
	}
	now := time.Now()
	h.mu.Lock()
	wait := now.Sub(ou.lastSearch) < searchCooldown
	if !wait {
		ou.lastSearch = now
	}
	h.mu.Unlock()
	if wait {
		cl.enqueue(errFrame("slow_down"))
		return
	}
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	rows, err := h.st.searchPublic(dctx, ou.id, q, searchLimit)
	if err != nil {
		log.Printf("people search failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, u := range rows {
		e := map[string]any{"h": u.Handle}
		if u.Avatar != "" {
			e["v"] = u.Avatar
		}
		if isStaff(u.Role) {
			e["o"] = u.Role
		} else {
			e["r"] = tierFor(u.Points)
		}
		out = append(out, e)
	}
	cl.enqueue(frame(map[string]any{"t": "ufound", "q": q, "r": out}))
}

func onlyHandleRunes(s string) bool {
	for _, r := range s {
		if !(r == '_' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

// ---- reports about people and rooms ----

// reportUser handles {"t":"urep","h":"bob","c":"harassment","b":"what happened"}.
func (h *hub) reportUser(ctx context.Context, cl *client, in inbound) {
	ou := cl.user
	if !validReportReason(in.C) {
		cl.enqueue(errFrame("bad_report"))
		return
	}
	details, ok := cleanDetails(in.B, maxDetailRunes)
	if !ok || (in.C == "other" && utf8.RuneCountInString(details) < 5) {
		cl.enqueue(errFrame("bad_details"))
		return
	}
	now := time.Now()
	if !h.reportBudget(ou, now) {
		cl.enqueue(errFrame("slow_down"))
		return
	}
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	target, ok := h.lookupUser(dctx, cl, in.H)
	if !ok {
		return
	}
	switch {
	case target.ID == ou.id:
		cl.enqueue(errFrame("self_report"))
		return
	case isStaff(target.Role):
		cl.enqueue(errFrame("staff_report")) // staff have their own channel
		return
	}
	_, dup, err := h.st.addTargetReport(dctx, reportInput{ReporterID: ou.id, ReporterHandle: ou.handle, Kind: "user",
		TargetUserID: target.ID, TargetHandle: target.Handle, Reason: in.C, Details: details})
	if err != nil {
		log.Printf("saving a user report failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	if dup {
		cl.enqueue(errFrame("already_reported"))
		return
	}
	h.reportSpent(ou, now)
	cl.enqueue(frame(map[string]string{"t": "repok"}))
	h.sendReportCount(dctx)
}

// reportRoom handles {"t":"rrep","r":"slug","c":"abuse","b":"..."}. Only rooms the person can see can be reported.
func (h *hub) reportRoom(ctx context.Context, cl *client, in inbound) {
	ou := cl.user
	if !validReportReason(in.C) {
		cl.enqueue(errFrame("bad_report"))
		return
	}
	details, ok := cleanDetails(in.B, maxDetailRunes)
	if !ok || (in.C == "other" && utf8.RuneCountInString(details) < 5) {
		cl.enqueue(errFrame("bad_details"))
		return
	}
	now := time.Now()
	if !h.reportBudget(ou, now) {
		cl.enqueue(errFrame("slow_down"))
		return
	}
	h.mu.Lock()
	rm := h.rooms[in.R]
	var slug, name, ownerHandle string
	var ownerID int64
	visible := false
	if rm != nil && rm.community() {
		_, mine := ou.rooms[rm.slug]
		visible = (rm.listed() || mine || rm.ownerID == ou.id || isStaff(ou.role)) && !(rm.ownerID != ou.id && rm.kind == "private" && !mine && !isStaff(ou.role))
		slug, name, ownerID, ownerHandle = rm.slug, rm.name, rm.ownerID, rm.ownerHandle
	}
	h.mu.Unlock()
	if !visible {
		cl.enqueue(errFrame("no_such_room"))
		return
	}
	if ownerID == ou.id {
		cl.enqueue(errFrame("self_report"))
		return
	}
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	if orole, err := h.st.roleOf(dctx, ownerID); err == nil && isStaff(orole) {
		// A room run by staff is reported to admins only.
		h.fileComplaint(dctx, cl, complaintRow{TargetID: ownerID, TargetHandle: ownerHandle, TargetRole: orole,
			Body: "[room] " + name, Category: complaintCategory(in.C), Details: details})
		return
	}
	_, dup, err := h.st.addTargetReport(dctx, reportInput{ReporterID: ou.id, ReporterHandle: ou.handle, Kind: "room",
		TargetRoom: slug, TargetHandle: ownerHandle, Reason: in.C, Details: details})
	if err != nil {
		log.Printf("saving a room report failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	if dup {
		cl.enqueue(errFrame("already_reported"))
		return
	}
	h.reportSpent(ou, now)
	cl.enqueue(frame(map[string]string{"t": "repok"}))
	h.sendReportCount(dctx)
}

// ---- complaints about staff ----

func complaintCategory(c string) string {
	if validStaffReason(c) {
		return c
	}
	return "other" // spam and scam do not apply to staff misconduct
}

// staffComplaint handles {"t":"scomp","h":"staffhandle","i":42,"c":"abuse","b":"what happened"}.
// It is the way to report an admin or moderator. Moderators can not read these, only admins.
func (h *hub) staffComplaint(ctx context.Context, cl *client, in inbound) {
	ou := cl.user
	if !validStaffReason(in.C) {
		cl.enqueue(errFrame("bad_report"))
		return
	}
	details, ok := cleanDetails(in.B, 500)
	if !ok || (in.C == "other" && utf8.RuneCountInString(details) < 10) {
		cl.enqueue(errFrame("bad_details"))
		return
	}
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	target, ok := h.lookupUser(dctx, cl, in.H)
	if !ok {
		return
	}
	switch {
	case target.ID == ou.id:
		cl.enqueue(errFrame("self_report"))
		return
	case !isStaff(target.Role):
		cl.enqueue(errFrame("not_staff")) // ordinary people are reported the ordinary way
		return
	}
	c := complaintRow{TargetID: target.ID, TargetHandle: target.Handle, TargetRole: target.Role, Category: in.C, Details: details}
	if in.I > 0 {
		h.mu.Lock()
		var m *chatMsg
		if cl.room != nil {
			m = findMsg(cl.room, in.I)
		}
		h.mu.Unlock()
		if m == nil || !strings.EqualFold(m.Handle, target.Handle) {
			cl.enqueue(errFrame("report_gone"))
			return
		}
		c.MsgID, c.Body = m.ID, m.Body
	}
	h.fileComplaint(dctx, cl, c)
}

// fileComplaint stores a complaint and tells every admin who is online. It sends the answer to the reporter.
func (h *hub) fileComplaint(ctx context.Context, cl *client, c complaintRow) {
	ou := cl.user
	now := time.Now()
	if !h.reportBudget(ou, now) {
		cl.enqueue(errFrame("slow_down"))
		return
	}
	c.ReporterID, c.ReporterHandle = ou.id, ou.handle
	c.Category = complaintCategory(c.Category)
	_, dup, err := h.st.addComplaint(ctx, c)
	if err != nil {
		log.Printf("saving a staff complaint failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	if dup {
		cl.enqueue(errFrame("already_reported"))
		return
	}
	h.reportSpent(ou, now)
	cl.enqueue(frame(map[string]string{"t": "scompok"}))
	h.sendComplaintCount(ctx)
}

func (h *hub) sendComplaintCount(ctx context.Context) {
	type adm struct {
		id int64
		ou *onlineUser
	}
	var admins []adm
	h.mu.Lock()
	for id, ou := range h.users {
		if ou.role == "admin" {
			admins = append(admins, adm{id, ou})
		}
	}
	h.mu.Unlock()
	for _, a := range admins {
		n, err := h.st.openComplaintCount(ctx, a.id)
		if err != nil {
			continue
		}
		h.sendToUser(a.id, frame(map[string]any{"t": "cn", "c": n}))
	}
}

// complaintAdmin handles {"t":"acomp"} (list) and {"t":"acompdone","id":3,"a":"resolved|dismissed","b":"note"}. Admins only.
func (h *hub) complaintAdmin(ctx context.Context, cl *client, in inbound) {
	if !h.isAdmin(cl.user) {
		cl.enqueue(errFrame("forbidden"))
		return
	}
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	if in.T == "acomp" {
		rows, err := h.st.openComplaints(dctx, cl.user.id, 50)
		if err != nil {
			log.Printf("listing complaints failed: %v", err)
			cl.enqueue(errFrame("server_busy"))
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, r := range rows {
			out = append(out, map[string]any{"id": r.ID, "by": r.ReporterHandle, "h": r.TargetHandle, "o": r.TargetRole,
				"i": r.MsgID, "b": r.Body, "c": r.Category, "d": r.Details, "ts": r.TS})
		}
		cl.enqueue(frame(map[string]any{"t": "comps", "r": out}))
		return
	}
	if in.A != "resolved" && in.A != "dismissed" {
		cl.enqueue(errFrame("bad_action"))
		return
	}
	note, ok := cleanDetails(in.B, 300)
	if !ok {
		cl.enqueue(errFrame("bad_details"))
		return
	}
	done, err := h.st.closeComplaint(dctx, in.ID, in.A, note, cl.user.handle, cl.user.id)
	if err != nil {
		log.Printf("closing a complaint failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	if !done {
		cl.enqueue(errFrame("report_gone"))
		return
	}
	cl.enqueue(frame(map[string]any{"t": "modok", "a": "complaint", "id": in.ID}))
	h.sendComplaintCount(dctx)
}

// ---- account deletion ----

// deleteRequest is step one: {"t":"delreq","p":"password"} for accounts with a password, {"t":"delreq","h":"myhandle"}
// for guests. It checks who is asking and hands back a one time token valid for a few minutes.
func (h *hub) deleteRequest(ctx context.Context, cl *client, in inbound) {
	ou := cl.user
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	row, err := h.st.userByKey(dctx, strings.ToLower(ou.handle))
	if err != nil || row.ID != ou.id {
		cl.enqueue(errFrame("session_expired"))
		return
	}
	if row.Role == "admin" {
		cl.enqueue(errFrame("admin_cannot_delete"))
		return
	}
	key := "del:" + strings.ToLower(ou.handle)
	now := time.Now()
	if row.PassHash.Valid {
		if !h.limit.allowed(key, now) {
			cl.enqueue(errFrame("too_many_attempts"))
			return
		}
		match, aerr := h.auth.verify(dctx, row.PassHash.String, in.P)
		if aerr != nil {
			cl.enqueue(errFrame("server_busy"))
			return
		}
		if !match {
			h.limit.fail(key, now)
			cl.enqueue(errFrame("bad_password"))
			return
		}
		h.limit.reset(key)
	} else if !strings.EqualFold(strings.TrimPrefix(strings.TrimSpace(in.H), "@"), ou.handle) {
		cl.enqueue(errFrame("bad_confirm")) // guests confirm by typing their handle
		return
	}
	token := newToken()
	h.mu.Lock()
	ou.delTok, ou.delExp = tokenHash(token), now.Add(deleteTokenTTL)
	h.mu.Unlock()
	cl.enqueue(frame(map[string]any{"t": "delready", "k": token, "x": int(deleteTokenTTL.Seconds())}))
}

// deleteConfirm is step two: {"t":"delgo","k":"<token>","c":"DELETE"}. The token works once.
func (h *hub) deleteConfirm(ctx context.Context, cl *client, in inbound) {
	ou := cl.user
	if in.C != deleteConfirmWord {
		cl.enqueue(errFrame("bad_confirm"))
		return
	}
	h.mu.Lock()
	okTok := ou.delTok != "" && time.Now().Before(ou.delExp) &&
		subtle.ConstantTimeCompare([]byte(ou.delTok), []byte(tokenHash(in.K))) == 1
	if okTok {
		ou.delTok = "" // one use
	}
	h.mu.Unlock()
	if !okTok {
		cl.enqueue(errFrame("delete_expired"))
		return
	}
	dctx, cancel := context.WithTimeout(ctx, 3*authWait)
	defer cancel()
	photoKey := h.st.photoKeyByID(dctx, ou.id)
	res, err := h.st.deleteAccount(dctx, ou.id)
	switch {
	case errors.Is(err, errAdminTarget):
		cl.enqueue(errFrame("admin_cannot_delete"))
		return
	case err != nil:
		log.Printf("deleting an account failed: %v", err) // the transaction rolled back: nothing changed, the person can try again
		cl.enqueue(errFrame("server_busy"))
		return
	}
	h.photoDropObjects(photoKey) // the picture goes with the account
	cl.enqueue(frame(map[string]any{"t": "delok", "n": len(res.ClosedRooms), "f": res.Forfeited}))
	h.purgeDeleted(ou.id, res)
}
