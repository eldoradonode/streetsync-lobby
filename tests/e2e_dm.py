import asyncio, json, os, signal, subprocess, time, urllib.request, websockets

PORT = 8099
URL = f"ws://localhost:{PORT}/ws"
DB = "postgres://lobby:lobby@localhost/lobbytest?sslmode=disable"
BASE = dict(os.environ, PORT=str(PORT), DATABASE_URL=DB, HASH_ITERATIONS="1000", REQUIRE_PASSWORD="0", FLUSH_MS="500",
            ADMIN_HANDLE="Boss", ADMIN_PASSWORD="boss password 1")
BIN = os.environ.get("LOBBY_BIN", os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "lobby-test"))
passed = failed = 0
proc = None

def check(name, cond, extra=""):
    global passed, failed
    if cond: passed += 1; print("PASS", name)
    else: failed += 1; print("FAIL", name, extra)

def sql(q):
    r = subprocess.run(["psql", "-h", "localhost", "-U", "lobby", "-d", "lobbytest", "-tAc", q],
                       env=dict(os.environ, PGPASSWORD="lobby"), capture_output=True, text=True)
    return r.stdout.strip()

def start():
    global proc
    proc = subprocess.Popen([BIN], env=BASE, stdout=open("/tmp/lobby_dm.log", "a"), stderr=subprocess.STDOUT)
    for _ in range(80):
        try: urllib.request.urlopen(f"http://localhost:{PORT}/healthz", timeout=1); return
        except Exception: time.sleep(0.1)
    raise SystemExit("server did not start")

def stop():
    proc.send_signal(signal.SIGTERM); proc.wait(timeout=15)

async def recv(ws, t=None, timeout=3):
    while True:
        m = json.loads(await asyncio.wait_for(ws.recv(), timeout))
        if t is None or m["t"] == t: return m

async def quiet(ws, t, wait=0.6):
    end = time.time() + wait
    while time.time() < end:
        try:
            m = json.loads(await asyncio.wait_for(ws.recv(), max(0.05, end - time.time())))
            if m["t"] == t: return False
        except asyncio.TimeoutError: break
    return True

async def drain(ws, wait=0.3):
    while True:
        try: await asyncio.wait_for(ws.recv(), wait)
        except asyncio.TimeoutError: return

async def closed(ws, wait=3):
    try:
        while True: await asyncio.wait_for(ws.recv(), wait)
    except websockets.ConnectionClosed: return True
    except asyncio.TimeoutError: return False

async def conn(handle=None, token=None, login=None, room=None, last=0):
    ws = await websockets.connect(URL)
    extra = {"r": room, "l": last} if room else {}
    if login: await ws.send(json.dumps(dict(t="login", h=login[0], p=login[1], **extra)))
    elif token: await ws.send(json.dumps(dict(t="hi", k=token, **extra)))
    else: await ws.send(json.dumps(dict(t="hi", h=handle, **extra)))
    return ws

async def enter(handle=None, token=None, login=None, room=None):
    ws = await conn(handle, token, login, room)
    ok = await recv(ws, "ok")
    rooms = await recv(ws, "rooms")
    if room:
        await recv(ws, "joined"); await recv(ws, "hist"); await recv(ws, "n")
    await drain(ws, 0.15)
    return ws, ok, rooms

async def join(ws, slug, last=0):
    await ws.send(json.dumps({"t": "join", "r": slug, "l": last}))

async def say(ws, body, **kw):
    await ws.send(json.dumps(dict(t="m", b=body, **kw)))


async def me_with(ws, credits, timeout=4):
    end = time.time() + timeout
    last = None
    while time.time() < end:
        try: m = json.loads(await asyncio.wait_for(ws.recv(), max(0.05, end - time.time())))
        except asyncio.TimeoutError: break
        if m["t"] == "me": last = m
        if m["t"] == "me" and m["c"] == credits: return m
    return last

async def person(handle):
    """Sign in a new guest and wait until the welcome credits have arrived."""
    ws = await conn(handle); ok = await recv(ws, "ok")
    await me_with(ws, 20)
    return ws, ok["k"]

def send(ws, **kw): return ws.send(json.dumps(kw))

async def err(ws, timeout=2):
    return (await recv(ws, "err", timeout))["c"]

async def inbox(ws):
    await send(ws, t="inbox"); return await recv(ws, "inbox")

async def badge(ws, timeout=3):
    return await recv(ws, "ib", timeout)

async def main():
    sql("TRUNCATE users, sessions, messages, sanctions, reports, mod_log, rooms RESTART IDENTITY CASCADE")
    start()
    ann, tann = await person("ann")
    bob, tbob = await person("bob")

    # ---- a stranger message costs the sender one credit
    await send(ann, t="dm", h="bob", b="hello bob")
    d1 = await recv(ann, "dm"); d2 = await recv(bob, "dm")
    check("the sender gets their own message back", d1["f"] == "ann" and d1["w"] == "bob" and d1["b"] == "hello bob" and d1["i"] > 0, d1)
    check("the recipient gets the message at once", d2["i"] == d1["i"] and d2["f"] == "ann" and d2["b"] == "hello bob", d2)
    m = await me_with(ann, 19)
    check("the sender paid 1 credit", m is not None and m["c"] == 19, m)
    check("the recipient paid nothing", sql("SELECT credits FROM users WHERE handle_key='bob'") == "20")
    check("the charge is in the ledger", sql("SELECT count(*) FROM credit_ledger WHERE kind='dm' AND amount=-1") == "1")
    check("the message is stored", sql("SELECT count(*) FROM dm_messages") == "1")

    # ---- inbox, thread, unread
    ib = await inbox(bob)
    check("the inbox lists the chat with one unread", len(ib["c"]) == 1 and ib["c"][0]["h"] == "ann" and ib["c"][0]["u"] == 1 and ib["c"][0]["b"] == "hello bob" and ib["c"][0]["m"] is False, ib["c"])
    check("the inbox has empty lists for a new person", ib["f"] == [] and ib["q"] == [] and ib["p"] == [] and ib["x"] == [], ib)
    await send(bob, t="thread", h="ann")
    th = await recv(bob, "thread")
    check("the person who was written to sees the thread priced at nothing", th["h"] == "ann" and th["s"] == "none" and th["c"] == 0 and len(th["m"]) == 1 and th["m"][0]["b"] == "hello bob", th)
    b = await badge(bob)
    check("opening it clears the unread badge", b["u"] == 0 and b["q"] == 0, b)

    # the person who was written to replies for free, the first sender keeps paying
    await send(bob, t="dm", h="ann", b="hi ann")
    await recv(bob, "dm"); await recv(ann, "dm")
    await asyncio.sleep(0.4)
    check("a reply costs the person who was written to nothing", sql("SELECT credits FROM users WHERE handle_key='bob'") == "20")
    await send(ann, t="thread", h="bob"); th = await recv(ann, "thread")
    check("the first sender still sees a price of 1", th["c"] == 1 and len(th["m"]) == 2, th)
    await send(ann, t="dm", h="bob", b="you there?")
    await recv(ann, "dm"); await recv(bob, "dm")
    m = await me_with(ann, 18)
    check("the first sender pays again for a second message", m is not None and m["c"] == 18, m)

    # ---- friend request and acceptance make it free
    await send(ann, t="fr", a="add", h="bob")
    r = await recv(ann, "frok")
    check("the request is confirmed as sent", r["a"] == "add" and r["h"] == "bob" and r["s"] == "sent", r)
    ev = await recv(bob, "fr")
    check("the other person is told about the request", ev["a"] == "request" and ev["h"] == "ann", ev)
    b = await badge(bob)
    while b["q"] != 1: b = await badge(bob)
    check("the request shows on their badge", b["q"] == 1, b)
    ib = await inbox(bob)
    check("it is in their request list", ib["q"] == ["ann"], ib)
    ib = await inbox(ann)
    check("and in the sender's pending list", ib["p"] == ["ann"] or ib["p"] == ["bob"], ib)
    await send(ann, t="fr", a="add", h="bob")
    check("asking twice is refused", await err(ann) == "already_friends")
    await send(ann, t="fr", a="accept", h="bob")
    check("you can not accept your own request", await err(ann) == "no_request")
    await send(bob, t="fr", a="accept", h="ann")
    r = await recv(bob, "frok")
    check("accepting makes them friends", r["s"] == "friend", r)
    ev = await recv(ann, "fr")
    check("the asker is told it was accepted", ev["a"] == "friends" and ev["h"] == "bob", ev)
    await send(ann, t="thread", h="bob")
    th = await recv(ann, "thread")
    check("the thread now says friends and costs nothing", th["s"] == "friend" and th["c"] == 0 and len(th["m"]) == 3, th)
    await send(ann, t="dm", h="bob", b="free now")
    await recv(ann, "dm"); await recv(bob, "dm")
    await asyncio.sleep(0.4)
    check("friends message for free", sql("SELECT credits FROM users WHERE handle_key='ann'") == "18" and sql("SELECT credits FROM users WHERE handle_key='bob'") == "20")
    await send(bob, t="dm", h="ann", b="free both ways")
    await recv(bob, "dm"); await recv(ann, "dm")
    await asyncio.sleep(0.3)
    check("and the other way", sql("SELECT credits FROM users WHERE handle_key='bob'") == "20")
    ib = await inbox(ann)
    check("friends are listed", ib["f"] == ["bob"], ib)

    # ---- refusals
    await send(ann, t="dm", h="ann", b="me"); check("a message to yourself", await err(ann) == "self_dm")
    await send(ann, t="dm", h="nobody", b="x"); check("a message to a missing person", await err(ann) == "no_such_user")
    await send(ann, t="dm", h="bob", b="   "); check("an empty message", await err(ann) == "empty")
    await send(ann, t="dm", h="bob", b="x" * 301); check("a message that is too long", await err(ann) == "too_long")
    await send(ann, t="dm", h="b", b="x"); check("a badly formed handle", await err(ann) == "no_such_user")
    await send(ann, t="fr", a="add", h="ann"); check("a request to yourself", await err(ann) == "self_dm")
    await send(ann, t="fr", a="dance", h="bob"); check("an unknown friend action", await err(ann) == "bad_action")
    await send(ann, t="fr", a="decline", h="bob"); check("declining nothing", await err(ann) == "no_request")
    await send(ann, t="blk", a="zap", h="bob"); check("an unknown block action", await err(ann) == "bad_action")

    # ---- no credits, no message
    cat, tcat = await person("cat")
    sql("INSERT INTO credit_ledger (user_id, kind, amount, balance_after, ref, note) SELECT id, 'spend', -20, 0, 'test-drain', '' FROM users WHERE handle_key='cat'")
    sql("UPDATE users SET credits=0 WHERE handle_key='cat'")
    await send(cat, t="dm", h="ann", b="i am broke")
    check("a stranger with no credits can not send", await err(cat) == "no_credits")
    check("nothing was stored for it", sql("SELECT count(*) FROM dm_messages WHERE body='i am broke'") == "0")
    await send(cat, t="fr", a="add", h="ann")
    r = await recv(cat, "frok")
    check("but a friend request is always free", r["s"] == "sent", r)
    await send(ann, t="fr", a="accept", h="cat"); await recv(ann, "frok")
    await send(cat, t="dm", h="ann", b="broke but friends")
    d = await recv(cat, "dm")
    check("and friends can talk with zero credits", d["b"] == "broke but friends", d)

    # ---- blocking
    await send(bob, t="blk", a="add", h="ann")
    r = await recv(bob, "blkok")
    check("blocking is confirmed", r["a"] == "add" and r["h"] == "ann", r)
    check("blocking ends the friendship", sql("SELECT count(*) FROM friendships f JOIN users u ON u.id=f.user_a OR u.id=f.user_b WHERE u.handle_key IN ('ann') AND f.status='accepted' AND (SELECT handle_key FROM users WHERE id = CASE WHEN f.user_a=u.id THEN f.user_b ELSE f.user_a END)='bob'") == "0")
    before = sql("SELECT credits FROM users WHERE handle_key='ann'")
    await send(ann, t="dm", h="bob", b="you there?")
    check("a blocked person gets a plain refusal", await err(ann) == "blocked")
    check("and is not charged", sql("SELECT credits FROM users WHERE handle_key='ann'") == before)
    await send(bob, t="dm", h="ann", b="i blocked you"); check("the blocker can not message either", await err(bob) == "blocked")
    await send(ann, t="fr", a="add", h="bob"); check("a blocked person can not ask", await err(ann) == "blocked")
    ib = await inbox(bob)
    check("the blocker's inbox hides that chat and lists the block", all(c["h"] != "ann" for c in ib["c"]) and ib["x"] == ["ann"], ib)
    await send(bob, t="thread", h="ann"); th = await recv(bob, "thread")
    check("the blocker sees the thread as blocked", th["s"] == "blocked", th)
    await send(bob, t="blk", a="del", h="ann"); await recv(bob, "blkok")
    await send(ann, t="dm", h="bob", b="peace?")
    d = await recv(ann, "dm")
    check("after unblocking, messages flow again", d["b"] == "peace?", d)
    await recv(bob, "dm")

    # ---- reporting a direct message, and a moderator removing it
    mid = d["i"]
    await send(ann, t="drep", i=mid, c="abuse"); check("you can not report a message you sent", await err(ann) == "report_gone")
    await send(bob, t="drep", i=mid, c="rude"); check("a bad reason is refused", await err(bob) == "bad_report")
    await send(bob, t="drep", i=999999, c="abuse"); check("a missing message", await err(bob) == "report_gone")
    await send(bob, t="drep", i=mid, c="abuse"); await recv(bob, "repok")
    await send(bob, t="drep", i=mid, c="abuse"); check("reporting twice", await err(bob) == "already_reported")
    boss = await conn(login=("Boss", "boss password 1")); await recv(boss, "ok"); await drain(boss, 0.3)
    await send(boss, t="reports"); reps = await recv(boss, "reps")
    rep = [x for x in reps["r"] if x["i"] == mid]
    check("the moderator sees it marked as a direct message", len(rep) == 1 and rep[0]["b"] == "[direct message] peace?" and rep[0]["h"] == "ann" and rep[0]["by"] == "bob", reps)
    await send(boss, t="mod", a="del", i=mid, rid=rep[0]["id"]); r = await recv(boss, "modok")
    check("the moderator can delete it", r["a"] == "del" and sql(f"SELECT count(*) FROM dm_messages WHERE id={mid}") == "0")
    check("and the report is closed", sql(f"SELECT status FROM reports WHERE id={rep[0]['id']}").startswith("actioned"), sql("SELECT status FROM reports"))
    check("the removal is in the mod log", sql("SELECT count(*) FROM mod_log WHERE action='del' AND target='ann' AND detail='direct message'") == "1")
    await send(ann, t="mod", a="del", i=d["i"] + 1000); e = await err(ann)
    check("a normal member can not delete messages by id", e in ("forbidden", "msg_gone"), e)

    # ---- staff are free, muted people are silent
    await send(boss, t="dm", h="dan", b="hi"); check("staff messaging a missing person", await err(boss) == "no_such_user")
    dan, tdan = await person("dan")
    await send(boss, t="dm", h="dan", b="welcome from the team")
    await recv(boss, "dm"); got = await recv(dan, "dm")
    check("staff can message anyone", got["f"] == "Boss" and got["b"] == "welcome from the team", got)
    await asyncio.sleep(0.3)
    check("and it costs staff nothing", sql("SELECT credits FROM users WHERE handle_key='boss'") == "20")
    await send(boss, t="mod", a="mute", h="cat", m=10); await recv(boss, "modok")
    await send(cat, t="dm", h="ann", b="muted?")
    check("a muted person can not send direct messages", await err(cat) == "muted")

    # ---- offline delivery and unread on return
    await dan.close()
    await asyncio.sleep(1.5)
    await send(ann, t="dm", h="dan", b="while you were away")
    await recv(ann, "dm")
    dan = await conn(token=tdan); await recv(dan, "ok")
    b = await badge(dan)
    check("someone who was away sees the unread count when they come back", b["u"] == 2 and b["q"] == 0, b)
    ib = await inbox(dan)
    check("both chats are waiting", {c["h"] for c in ib["c"]} == {"Boss", "ann"} and sum(c["u"] for c in ib["c"]) == 2, ib["c"])
    await send(dan, t="read", h="ann"); b = await badge(dan)
    check("marking read lowers the badge", b["u"] == 1, b)

    # ---- everything survives a restart
    for ws in (ann, bob, cat, dan, boss): await ws.close()
    await asyncio.sleep(1.0)
    stop(); start()
    ann = await conn(token=tann); await recv(ann, "ok")
    ib = await inbox(ann)
    check("chats survive a restart", {c["h"] for c in ib["c"]} == {"bob", "cat", "dan"}, ib["c"])
    await send(ann, t="thread", h="bob"); th = await recv(ann, "thread")
    check("history survives a restart", [m["b"] for m in th["m"]][:3] == ["hello bob", "hi ann", "you there?"], th["m"])
    await send(ann, t="dm", h="bob", b="after restart")
    d = await recv(ann, "dm")
    check("new messages get fresh ids after a restart", d["i"] > max(m["i"] for m in th["m"]), d)
    off = sql("SELECT count(*) FROM users u WHERE credits <> coalesce((SELECT sum(amount) FROM credit_ledger l WHERE l.user_id=u.id),0)")
    check("every balance still equals its ledger", off == "0", off)
    await ann.close()
    stop()
    print(f"\n{passed} passed, {failed} failed")
    raise SystemExit(1 if failed else 0)

asyncio.run(main())
