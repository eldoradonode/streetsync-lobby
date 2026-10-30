package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lib/pq"
)

// An avatar look is a letter for the style set (m, f or x) and a number from 0 to 7, like "f3".
// The page draws it. Nothing about gender is stored, only which picture was chosen.
var avatarRE = regexp.MustCompile(`^[mfx][0-7]$`)

// cleanAvatar returns the look if it is a real one, and an empty string otherwise.
func cleanAvatar(v string) string {
	if avatarRE.MatchString(v) {
		return v
	}
	return ""
}

func (s *store) setAvatar(ctx context.Context, userID int64, look string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET avatar = $2 WHERE id = $1`, userID, look)
	return err
}

// avatarsFor gives the picked looks of the people with these handles. People who never picked one are left out.
func (s *store) avatarsFor(ctx context.Context, handles []string) (map[string]string, error) {
	out := map[string]string{}
	if len(handles) == 0 {
		return out, nil
	}
	keys := make([]string, 0, len(handles))
	for _, h := range handles {
		if _, k, ok := normHandle(h); ok {
			keys = append(keys, k)
		}
	}
	rows, err := s.db.QueryContext(ctx, `SELECT u.handle, u.avatar || `+photoLookSQL+` FROM users u WHERE u.handle_key = ANY($1) AND (u.avatar <> '' OR u.photo_key IS NOT NULL)`, pq.Array(keys))
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var handle, look string
		if err := rows.Scan(&handle, &look); err != nil {
			return out, err
		}
		out[handle] = look
	}
	return out, rows.Err()
}

type profileInfo struct {
	Handle string
	Avatar string
	Bio    string
	Points int64
	Role   string
	Joined time.Time
}

func (s *store) profileOf(ctx context.Context, key string) (profileInfo, int64, error) {
	var p profileInfo
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id, handle, avatar, bio, points, role, created_at FROM users WHERE handle_key = $1 AND deleted_at IS NULL`, key).
		Scan(&id, &p.Handle, &p.Avatar, &p.Bio, &p.Points, &p.Role, &p.Joined)
	if errors.Is(err, sql.ErrNoRows) {
		err = errNotFound
	}
	return p, id, err
}

// setAvatar handles {"t":"av","c":"f3"}: change my look. All my tabs get the new look in a me frame.
func (h *hub) setAvatar(ctx context.Context, cl *client, in inbound) {
	look := cleanAvatar(in.C)
	if look == "" {
		cl.enqueue(errFrame("bad_avatar"))
		return
	}
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	ou := cl.user
	if err := h.st.setAvatar(dctx, ou.id, look); err != nil {
		log.Printf("saving an avatar failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	h.mu.Lock()
	ou.avatar = look
	f := frame(meFrame{T: "me", meFields: ou.me()})
	for c := range ou.clients {
		c.enqueue(f)
	}
	h.mu.Unlock()
}

// profile handles {"t":"profile","h":"bob"}: what anyone may see about a person, and how I stand with them.
func (h *hub) profile(ctx context.Context, cl *client, in inbound) {
	_, key, ok := normHandle(in.H)
	if !ok {
		cl.enqueue(errFrame("no_such_user"))
		return
	}
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	p, id, err := h.st.profileOf(dctx, key)
	if errors.Is(err, errNotFound) {
		cl.enqueue(errFrame("no_such_user"))
		return
	}
	var rel string
	var blockedMe, banned bool
	if err == nil && id != cl.user.id {
		rel, err = h.relationFor(dctx, cl.user.id, id)
		if err == nil {
			blockedMe, banned, err = h.st.hiddenFrom(dctx, cl.user.id, id)
		}
	}
	if err != nil {
		log.Printf("loading a profile failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	h.mu.Lock()
	ou := h.users[id]
	online := ou != nil && len(ou.clients) > 0
	viewerRole := cl.user.role
	viewerStaff := roleRank(viewerRole) >= 2
	h.mu.Unlock()
	_ = blockedMe // a person who blocked me is not revealed: the profile shows as before
	if !viewerStaff && banned {
		cl.enqueue(errFrame("no_such_user")) // banned accounts do not show up
		return
	}
	if online && id != cl.user.id && !viewerStaff && h.prefsOf(dctx, id).HideOnline {
		online = false // they chose not to show when they are online
	}
	out := map[string]any{"t": "uprofile", "h": p.Handle, "j": p.Joined.UnixMilli(), "on": online}
	if p.Avatar != "" {
		out["v"] = p.Avatar
	}
	if p.Bio != "" {
		out["bio"] = p.Bio
	}
	if rel != "" {
		out["s"] = rel
	}
	if id != cl.user.id && staffShielded(viewerRole, p.Role) {
		// The page hides tip, message, friend and report buttons for staff. The server refuses them regardless.
		out["sh"] = true
		if started, serr := staffStartedWith(dctx, h.st.db, cl.user.id, id); serr == nil && started {
			out["dmok"] = true // the staff member wrote first, so a reply is allowed
		}
	}
	if viewerStaff && banned {
		out["ban"] = true
	}
	if p.Role == "mod" || p.Role == "admin" {
		out["o"] = p.Role // staff show a badge instead of a tier and points
	} else {
		out["r"] = tierFor(p.Points)
		out["p"] = p.Points
	}
	cl.enqueue(frame(out))
}

const maxBioRunes = 160

// cleanBio tidies a bio: no line breaks or control characters, spaces squeezed, at most 160 characters.
// ok is false when it is too long, so people see it was not saved instead of silently losing words.
func cleanBio(raw string) (string, bool) {
	s := strings.Join(strings.Fields(clean(raw)), " ")
	if utf8.RuneCountInString(s) > maxBioRunes {
		return "", false
	}
	return s, true
}

func (s *store) setBio(ctx context.Context, userID int64, bio string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET bio = $2 WHERE id = $1`, userID, bio)
	return err
}

// setBio handles {"t":"bio","b":"..."}: change my own bio. An empty one clears it.
func (h *hub) setBio(ctx context.Context, cl *client, in inbound) {
	bio, ok := cleanBio(in.B)
	if !ok {
		cl.enqueue(errFrame("bad_bio"))
		return
	}
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	ou := cl.user
	if err := h.st.setBio(dctx, ou.id, bio); err != nil {
		log.Printf("saving a bio failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	h.mu.Lock()
	ou.bio = bio
	f := frame(meFrame{T: "me", meFields: ou.me()})
	for c := range ou.clients {
		c.enqueue(f)
	}
	h.mu.Unlock()
	cl.enqueue(frame(map[string]string{"t": "biook"}))
}
