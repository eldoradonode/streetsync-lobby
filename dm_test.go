package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func mkUsers(t *testing.T, st *store, names ...string) []userRow {
	t.Helper()
	out := []userRow{}
	for _, n := range names {
		u, _, err := st.createGuest(context.Background(), n, n)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, u)
	}
	return out
}

func credits(t *testing.T, st *store, id int64) int64 {
	t.Helper()
	b, err := st.creditBalance(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestStrangerPaysFriendDoesNot(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u := mkUsers(t, st, "t_dma", "t_dmb", "t_dmx")
	a, b := u[0], u[1]
	if _, _, err := st.applyCredit(ctx, a.ID, "grant", 3, "g", ""); err != nil {
		t.Fatal(err)
	}

	bal, charged, err := st.sendDM(ctx, 1001, a.ID, b.ID, b.Handle, "hello stranger", 1_700_000_000_000, 1)
	if err != nil || !charged || bal != 2 {
		t.Fatalf("stranger message: bal=%d charged=%v err=%v", bal, charged, err)
	}
	if credits(t, st, b.ID) != 0 {
		t.Error("the recipient must never pay")
	}
	// The person who was written to replies for free, even with no credits at all.
	if cost, _ := st.dmCost(ctx, b.ID, a.ID, 1); cost != 0 {
		t.Errorf("the reply is priced at %d, want 0", cost)
	}
	if _, charged, err := st.sendDM(ctx, 1002, b.ID, a.ID, a.Handle, "hi back", 1_700_000_000_001, 1); err != nil || charged {
		t.Fatalf("a reply must be free: charged=%v err=%v", charged, err)
	}
	if credits(t, st, b.ID) != 0 {
		t.Error("the replier must not pay")
	}
	// The first sender keeps paying for every message while they are not friends.
	if cost, _ := st.dmCost(ctx, a.ID, b.ID, 1); cost != 1 {
		t.Errorf("the first sender is priced at %d, want 1", cost)
	}
	if bal, charged, err := st.sendDM(ctx, 1010, a.ID, b.ID, b.Handle, "second from a", 1_700_000_000_005, 1); err != nil || !charged || bal != 1 {
		t.Fatalf("second message from the first sender: bal=%d charged=%v err=%v", bal, charged, err)
	}
	// And someone with no credits can not start a conversation.
	if _, _, err := st.sendDM(ctx, 1011, b.ID, u[2].ID, u[2].Handle, "cold message", 1_700_000_000_006, 1); !errors.Is(err, errNoFunds) {
		t.Fatalf("a stranger with no credits must not be able to start a chat, got %v", err)
	}
	msgs, _ := st.threadMessages(ctx, b.ID, u[2].ID, 50)
	if len(msgs) != 0 {
		t.Fatalf("a refused message must not be stored: %+v", msgs)
	}

	// Become friends: both directions are now free.
	if rel, err := st.friendAction(ctx, "add", a.ID, b.ID); err != nil || rel != "sent" {
		t.Fatalf("add: %q %v", rel, err)
	}
	if rel, err := st.friendAction(ctx, "accept", b.ID, a.ID); err != nil || rel != "friend" {
		t.Fatalf("accept: %q %v", rel, err)
	}
	if bal, charged, err := st.sendDM(ctx, 1003, a.ID, b.ID, b.Handle, "now free", 1_700_000_000_002, 1); err != nil || charged || bal != 0 {
		t.Fatalf("friend message: bal=%d charged=%v err=%v", bal, charged, err)
	}
	if _, charged, err := st.sendDM(ctx, 1004, b.ID, a.ID, a.Handle, "free too", 1_700_000_000_003, 1); err != nil || charged {
		t.Fatalf("friend reply: charged=%v err=%v", charged, err)
	}
	if credits(t, st, a.ID) != 1 {
		t.Errorf("friends must not be charged, balance %d", credits(t, st, a.ID))
	}
	// Ending the friendship makes it cost again.
	if rel, err := st.friendAction(ctx, "remove", b.ID, a.ID); err != nil || rel != "none" {
		t.Fatalf("remove: %q %v", rel, err)
	}
	if _, charged, _ := st.sendDM(ctx, 1005, a.ID, b.ID, b.Handle, "stranger again", 1_700_000_000_004, 1); !charged {
		t.Error("after unfriending the first sender pays again")
	}
	if _, charged, _ := st.sendDM(ctx, 1006, b.ID, a.ID, a.Handle, "reply again", 1_700_000_000_005, 1); charged {
		t.Error("after unfriending the person who was written to still replies free")
	}
}

func TestSameMessageIdNeverChargesTwice(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u := mkUsers(t, st, "t_dmc", "t_dmd")
	st.applyCredit(ctx, u[0].ID, "grant", 5, "g", "")
	if _, _, err := st.sendDM(ctx, 2001, u[0].ID, u[1].ID, u[1].Handle, "once", 1, 1); err != nil {
		t.Fatal(err)
	}
	// A retry with the same id fails on the message key and rolls the charge back with it.
	if _, _, err := st.sendDM(ctx, 2001, u[0].ID, u[1].ID, u[1].Handle, "once", 1, 1); err == nil {
		t.Fatal("a repeated id must be refused")
	}
	if got := credits(t, st, u[0].ID); got != 4 {
		t.Errorf("balance %d, want 4 (charged once)", got)
	}
}

func TestFriendRules(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u := mkUsers(t, st, "t_fra", "t_frb", "t_frc")
	a, b, c := u[0], u[1], u[2]

	if _, err := st.friendAction(ctx, "accept", a.ID, b.ID); !errors.Is(err, errNoRequest) {
		t.Errorf("accepting nothing: %v", err)
	}
	if _, err := st.friendAction(ctx, "add", a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.friendAction(ctx, "add", a.ID, b.ID); !errors.Is(err, errAlready) {
		t.Errorf("asking twice: %v", err)
	}
	if _, err := st.friendAction(ctx, "accept", a.ID, b.ID); !errors.Is(err, errNoRequest) {
		t.Errorf("the asker can not accept their own request: %v", err)
	}
	if rel, _ := st.relation(ctx, a.ID, b.ID); rel != "sent" {
		t.Errorf("a sees %q", rel)
	}
	if rel, _ := st.relation(ctx, b.ID, a.ID); rel != "got" {
		t.Errorf("b sees %q", rel)
	}
	// Cancel and decline both remove the request.
	if rel, err := st.friendAction(ctx, "cancel", a.ID, b.ID); err != nil || rel != "none" {
		t.Fatalf("cancel: %q %v", rel, err)
	}
	st.friendAction(ctx, "add", a.ID, b.ID)
	if _, err := st.friendAction(ctx, "cancel", b.ID, a.ID); !errors.Is(err, errNoRequest) {
		t.Errorf("only the asker can cancel: %v", err)
	}
	if rel, err := st.friendAction(ctx, "decline", b.ID, a.ID); err != nil || rel != "none" {
		t.Fatalf("decline: %q %v", rel, err)
	}
	// Two people asking each other at the same time become friends at once.
	st.friendAction(ctx, "add", a.ID, b.ID)
	if rel, err := st.friendAction(ctx, "add", b.ID, a.ID); err != nil || rel != "friend" {
		t.Fatalf("crossing requests: %q %v", rel, err)
	}
	if _, err := st.friendAction(ctx, "add", a.ID, b.ID); !errors.Is(err, errAlready) {
		t.Errorf("asking a friend: %v", err)
	}
	// Lists.
	st.friendAction(ctx, "add", c.ID, a.ID)
	l, err := st.friendLists(ctx, a.ID)
	if err != nil || len(l.Friends) != 1 || l.Friends[0] != b.Handle || len(l.Incoming) != 1 || l.Incoming[0] != c.Handle || len(l.Outgoing) != 0 {
		t.Fatalf("lists for a: %+v %v", l, err)
	}
	lc, _ := st.friendLists(ctx, c.ID)
	if len(lc.Outgoing) != 1 || lc.Outgoing[0] != a.Handle {
		t.Errorf("lists for c: %+v", lc)
	}
	if _, q, _ := st.unreadAndRequests(ctx, a.ID); q != 1 {
		t.Errorf("a has %d waiting requests, want 1", q)
	}
	if _, q, _ := st.unreadAndRequests(ctx, c.ID); q != 0 {
		t.Errorf("the asker has %d waiting requests, want 0", q)
	}
}

func TestBlocking(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u := mkUsers(t, st, "t_bla", "t_blb")
	a, b := u[0], u[1]
	st.applyCredit(ctx, a.ID, "grant", 5, "g", "")
	st.applyCredit(ctx, b.ID, "grant", 5, "g", "")
	st.friendAction(ctx, "add", a.ID, b.ID)
	st.friendAction(ctx, "accept", b.ID, a.ID)
	if _, _, err := st.sendDM(ctx, 3001, b.ID, a.ID, a.Handle, "before the block", 1, 1); err != nil {
		t.Fatal(err)
	}

	if err := st.blockUser(ctx, a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	if rel, _ := st.relation(ctx, a.ID, b.ID); rel != "none" {
		t.Errorf("blocking must end the friendship, got %q", rel)
	}
	// Both directions are shut, and nothing is charged for a refused message.
	for i, pr := range [][2]userRow{{a, b}, {b, a}} {
		before := credits(t, st, pr[0].ID)
		if _, _, err := st.sendDM(ctx, int64(3010+i), pr[0].ID, pr[1].ID, pr[1].Handle, "blocked", 1, 1); !errors.Is(err, errBlocked) {
			t.Errorf("message %d: %v", i, err)
		}
		if credits(t, st, pr[0].ID) != before {
			t.Errorf("a refused message must not charge (%d)", i)
		}
	}
	if _, err := st.friendAction(ctx, "add", b.ID, a.ID); !errors.Is(err, errBlocked) {
		t.Errorf("a blocked person can not send a request: %v", err)
	}
	if _, err := st.friendAction(ctx, "add", a.ID, b.ID); !errors.Is(err, errBlocked) {
		t.Errorf("the blocker can not send one either until they unblock: %v", err)
	}
	// The blocker no longer sees the chat, and the unread count ignores the blocked person.
	chats, _ := st.conversations(ctx, a.ID)
	if len(chats) != 0 {
		t.Errorf("blocked chat still listed: %+v", chats)
	}
	if u, _, _ := st.unreadAndRequests(ctx, a.ID); u != 0 {
		t.Errorf("unread from a blocked person counts: %d", u)
	}
	if ok, _ := st.iBlocked(ctx, a.ID, b.ID); !ok {
		t.Error("iBlocked says no")
	}
	if ok, _ := st.iBlocked(ctx, b.ID, a.ID); ok {
		t.Error("the blocked person did not block anyone")
	}
	l, _ := st.friendLists(ctx, a.ID)
	if len(l.Blocked) != 1 || l.Blocked[0] != b.Handle {
		t.Errorf("blocked list: %+v", l.Blocked)
	}
	// Unblocking opens it again (strangers now, so it costs).
	if err := st.unblockUser(ctx, a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	// b sent the first message of this conversation, so after the unblock b pays and a replies free.
	if _, charged, err := st.sendDM(ctx, 3020, a.ID, b.ID, b.Handle, "hello again", 1, 1); err != nil || charged {
		t.Errorf("after unblock, the person who was written to: charged=%v err=%v", charged, err)
	}
	if _, charged, err := st.sendDM(ctx, 3021, b.ID, a.ID, a.Handle, "and from the first sender", 1, 1); err != nil || !charged {
		t.Errorf("after unblock, the first sender: charged=%v err=%v", charged, err)
	}
}

func TestInboxAndUnread(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u := mkUsers(t, st, "t_iba", "t_ibb", "t_ibc")
	a, b, c := u[0], u[1], u[2]
	for _, x := range []userRow{b, c} {
		st.applyCredit(ctx, x.ID, "grant", 10, "g", "")
	}
	st.sendDM(ctx, 4001, b.ID, a.ID, a.Handle, "first from b", 1000, 1)
	st.sendDM(ctx, 4002, b.ID, a.ID, a.Handle, "second from b", 2000, 1)
	st.sendDM(ctx, 4003, c.ID, a.ID, a.Handle, "from c", 3000, 1)

	if un, _, _ := st.unreadAndRequests(ctx, a.ID); un != 3 {
		t.Errorf("unread total %d, want 3", un)
	}
	chats, err := st.conversations(ctx, a.ID)
	if err != nil || len(chats) != 2 {
		t.Fatalf("chats: %+v %v", chats, err)
	}
	if chats[0].Handle != c.Handle || chats[0].Unread != 1 || chats[0].Mine || chats[0].Body != "from c" {
		t.Errorf("newest chat: %+v", chats[0])
	}
	if chats[1].Handle != b.Handle || chats[1].Unread != 2 || chats[1].Body != "second from b" {
		t.Errorf("older chat: %+v", chats[1])
	}
	// The sender sees the same chat with their own message last and nothing unread.
	bc, _ := st.conversations(ctx, b.ID)
	if len(bc) != 1 || !bc[0].Mine || bc[0].Unread != 0 || bc[0].Handle != a.Handle {
		t.Errorf("b's chats: %+v", bc)
	}
	// Opening a thread marks only that person's messages read.
	if err := st.markRead(ctx, a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	if un, _, _ := st.unreadAndRequests(ctx, a.ID); un != 1 {
		t.Errorf("unread after reading b: %d, want 1", un)
	}
	// Messages come back oldest first and respect the limit.
	msgs, _ := st.threadMessages(ctx, a.ID, b.ID, 50)
	if len(msgs) != 2 || msgs[0].Body != "first from b" || msgs[1].Body != "second from b" || msgs[0].TS != 1000 {
		t.Errorf("thread: %+v", msgs)
	}
	one, _ := st.threadMessages(ctx, a.ID, b.ID, 1)
	if len(one) != 1 || one[0].Body != "second from b" {
		t.Errorf("limit 1 must keep the newest: %+v", one)
	}
	// The thread works the same from either side.
	other, _ := st.threadMessages(ctx, b.ID, a.ID, 50)
	if len(other) != 2 {
		t.Errorf("thread from the other side: %+v", other)
	}
}

func TestDMReportDeleteAndPrune(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u := mkUsers(t, st, "t_dra", "t_drb")
	st.applyCredit(ctx, u[0].ID, "grant", 10, "g", "")
	st.sendDM(ctx, 5001, u[0].ID, u[1].ID, u[1].Handle, "bad words", 1, 1)
	h, sid, rid, body, err := st.dmByID(ctx, 5001)
	if err != nil || h != u[0].Handle || sid != u[0].ID || rid != u[1].ID || body != "bad words" {
		t.Fatalf("dmByID: %q %d %d %q %v", h, sid, rid, body, err)
	}
	if _, _, _, _, err := st.dmByID(ctx, 99999); !errors.Is(err, errNotFound) {
		t.Errorf("missing message: %v", err)
	}
	if who, err := st.deleteDM(ctx, 5001); err != nil || who != u[0].Handle {
		t.Errorf("deleteDM: %q %v", who, err)
	}
	if who, _ := st.deleteDM(ctx, 5001); who != "" {
		t.Errorf("deleting twice: %q", who)
	}
	// The id counter at start-up must see direct messages too.
	st.sendDM(ctx, 9_000_000_000, u[0].ID, u[1].ID, u[1].Handle, "big id", 1, 1)
	if max, _ := st.maxMessageID(ctx); max < 9_000_000_000 {
		t.Errorf("maxMessageID ignores direct messages: %d", max)
	}
	// Old messages are removed by the cleanup, new ones stay.
	st.sendDM(ctx, 5002, u[0].ID, u[1].ID, u[1].Handle, "fresh", time.Now().UnixMilli(), 1)
	st.db.Exec(`UPDATE dm_messages SET created_at = now() - interval '40 days' WHERE id = 9000000000`)
	n, err := st.pruneDMs(ctx, 30)
	if err != nil || n < 1 {
		t.Fatalf("prune: %d %v", n, err)
	}
	left, _ := st.threadMessages(ctx, u[0].ID, u[1].ID, 50)
	if len(left) != 1 || left[0].Body != "fresh" {
		t.Errorf("after prune: %+v", left)
	}
}

func TestFriendCap(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u := mkUsers(t, st, "t_cap0", "t_cap1", "t_cap2")
	// Fill person 0's friend list directly, with the ids of people who exist.
	for i := 0; i < friendCap-1; i++ {
		x, _, err := st.createGuest(ctx, "t_capx"+string(rune('a'+i/26))+string(rune('a'+i%26)), "t_capx"+string(rune('a'+i/26))+string(rune('a'+i%26)))
		if err != nil {
			t.Fatal(err)
		}
		lo, hi := pair(u[0].ID, x.ID)
		if _, err := st.db.Exec(`INSERT INTO friendships (user_a, user_b, requester, status) VALUES ($1,$2,$3,'accepted')`, lo, hi, x.ID); err != nil {
			t.Fatal(err)
		}
	}
	st.friendAction(ctx, "add", u[1].ID, u[0].ID)
	if rel, err := st.friendAction(ctx, "accept", u[0].ID, u[1].ID); err != nil || rel != "friend" {
		t.Fatalf("the 100th friend must fit: %q %v", rel, err)
	}
	st.friendAction(ctx, "add", u[2].ID, u[0].ID)
	if _, err := st.friendAction(ctx, "accept", u[0].ID, u[2].ID); !errors.Is(err, errFriendCap) {
		t.Errorf("the 101st friend must be refused: %v", err)
	}
	if rel, _ := st.relation(ctx, u[0].ID, u[2].ID); rel != "got" {
		t.Errorf("the request stays pending after a refused accept, got %q", rel)
	}
}
