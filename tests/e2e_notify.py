import asyncio, json, os, signal, subprocess, time, urllib.request, websockets

PORT = 8112
URL = f"ws://localhost:{PORT}/ws"
DB = "postgres://lobby:lobby@localhost/lobbytest?sslmode=disable"
BASE = dict(os.environ, PORT=str(PORT), DATABASE_URL=DB, HASH_ITERATIONS="1000", REQUIRE_PASSWORD="0", FLUSH_MS="500")
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

def start(extra=None):
    global proc
    env = dict(BASE, **(extra or {}))
    proc = subprocess.Popen([BIN], env=env, stdout=open("/tmp/lobby_notify.log", "a"), stderr=subprocess.STDOUT)
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

async def recv_me(ws, pred, timeout=3):
    """Next me frame that satisfies pred. Credit balance updates also arrive as me frames, at their own pace."""
    end = time.time() + timeout
    while True:
        m = json.loads(await asyncio.wait_for(ws.recv(), max(0.05, end - time.time())))
        if m["t"] == "me" and pred(m): return m

async def drain(ws, wait=0.3):
    while True:
        try: await asyncio.wait_for(ws.recv(), wait)
        except asyncio.TimeoutError: return

async def closed(ws, wait=3):
    try:
        while True: await asyncio.wait_for(ws.recv(), wait)
    except websockets.ConnectionClosed: return True
    except asyncio.TimeoutError: return False

async def join(handle=None, token=None, login=None):
    ws = await websockets.connect(URL)
    if login: await ws.send(json.dumps({"t": "login", "h": login[0], "p": login[1], "r": "lobby"}))
    elif token: await ws.send(json.dumps({"t": "hi", "k": token, "r": "lobby"}))
    else: await ws.send(json.dumps({"t": "hi", "h": handle, "r": "lobby"}))
    return ws

async def ready(ws):
    ok = await recv(ws, "ok"); await recv(ws, "hist"); await drain(ws, 0.2); return ok

async def say(ws, body, **kw):
    await ws.send(json.dumps(dict(t="m", b=body, **kw)))

async def mod(ws, **kw):
    await ws.send(json.dumps(dict(t="mod", **kw)))

async def send(ws, **kw): await ws.send(json.dumps(kw))
async def ask(ws, t, expect=None, **kw):
    await send(ws, t=t, **kw)
    m = await recv(ws, None, 3)
    while m["t"] not in (expect or t, "err"): m = await recv(ws, None, 3)
    return m
async def forbidden(ws, t, **kw):
    m = await ask(ws, t, **kw); return m["t"] == "err" and m["c"] == "forbidden"
async def go_pw(ws, pw):
    await send(ws, t="pw", p=pw); await recv(ws, "pwok"); await drain(ws, 0.2)

ENV = {"ADMIN_HANDLE": "Boss", "ADMIN_PASSWORD": "boss password 1"}

async def ready_n(ws):
    # like ready, but keeps what arrives at sign in: the settings (pf) and the unread count (nc)
    ok = await recv(ws, "ok"); await recv(ws, "hist"); got = {}
    end = time.time() + 0.6
    while time.time() < end:
        try: m = json.loads(await asyncio.wait_for(ws.recv(), max(0.05, end - time.time())))
        except asyncio.TimeoutError: break
        got[m["t"]] = m
    return ok, got.get("pf"), (got.get("nc") or {}).get("c")

async def nl(ws):
    await send(ws, t="nlist"); m = await recv(ws, "notes"); return m["l"], m["c"]
async def nn(ws, kind=None, timeout=3):
    end = time.time() + timeout
    while True:
        m = json.loads(await asyncio.wait_for(ws.recv(), max(0.05, end - time.time())))
        if m["t"] == "nn" and (kind is None or m["n"]["k"] == kind): return m
async def count(ws, timeout=3):
    m = await recv(ws, "nc", timeout); return m["c"]
async def set_pref(ws, k, b):
    await send(ws, t="set", k=k, b=b); return await recv(ws, "pf")
async def silent(ws, t="nn", wait=0.8): return await quiet(ws, t, wait)

async def main():
    sql("TRUNCATE users, sessions, messages, sanctions, reports, mod_log RESTART IDENTITY CASCADE")
    start(ENV)
    ann = await join("ann"); oka = await ready(ann); await go_pw(ann, "ann password 1")
    bob = await join("bob"); okb = await ready(bob); await go_pw(bob, "bob password 1")
    cleo = await join("cleo"); await ready(cleo)
    dan = await join("dan"); await ready(dan)
    boss = await join(login=("boss", "boss password 1")); await ready(boss)
    for w in (cleo, dan, boss):
        await send(w, t="join", r="football"); await recv(w, "joined")   # not in the room ann writes in
    for w in (ann, bob, cleo, dan, boss): await drain(w, 0.3)

    # ---- sign in gives the unread count and the settings
    await bob.close(); await asyncio.sleep(0.3)
    bob = await join(token=okb["k"]); _, pf, c0 = await ready_n(bob)
    check("sign in brings default settings", pf["nd"] and pf["nk"] and pf["nt"] and pf["nf"] and pf["dp"] == "everyone" and pf["so"] is True, pf)
    check("and a zero unread count", c0 == 0)

    # ---- direct messages: one grouped notification per person, with a count
    await send(bob, t="join", r="abuja"); await recv(bob, "joined")
    await send(ann, t="dm", h="bob", b="hello bob"); await recv(ann, "dm")
    n1 = await nn(bob, "dm")
    check("a direct message makes a notification for the receiver", n1["n"]["a"] == "ann" and n1["n"]["b"] == "hello bob" and n1["c"] == 1 and n1["n"].get("u") == 1, n1)
    check("the sender gets none", await silent(ann))
    await send(ann, t="dm", h="bob", b="you there?"); await recv(ann, "dm")
    n2 = await nn(bob, "dm")
    check("a second message groups into the same row with a count", n2["n"]["i"] == n1["n"]["i"] and n2["n"]["n"] == 2 and n2["c"] == 1 and n2["n"]["b"] == "you there?", n2)
    lst, c = await nl(bob)
    check("the list shows one unread row", c == 1 and len(lst) == 1 and lst[0]["k"] == "dm" and lst[0]["n"] == 2 and lst[0].get("u") == 1, lst)
    await send(bob, t="thread", h="ann"); await recv(bob, "thread")
    check("opening the conversation marks its notification read", await count(bob) == 0)
    lst, c = await nl(bob); check("and the row stays, now read", c == 0 and len(lst) == 1 and "u" not in lst[0])

    # ---- offline: it waits until the next sign in
    await bob.close(); await asyncio.sleep(0.3)
    await send(ann, t="dm", h="bob", b="while you were out"); await recv(ann, "dm")
    await asyncio.sleep(0.5)
    bob = await join(token=okb["k"]); _, _, c1 = await ready_n(bob)
    check("a message sent while offline is waiting at sign in", c1 == 1, c1)
    await send(bob, t="join", r="abuja"); await recv(bob, "joined")
    lst, c = await nl(bob); check("it reopened the read row with a fresh count of one", lst[0]["k"] == "dm" and lst[0]["n"] == 1 and lst[0].get("u") == 1, lst[0])

    # ---- tags and replies in rooms
    await say(ann, "hey @bob and @cleo nice one"); ma = await recv(ann, "m")
    t1 = await nn(bob, "tag")
    check("a tag in a room notifies the tagged person with the room name", t1["n"]["a"] == "ann" and t1["n"]["rn"] == "General Chat" and t1["n"]["r"] == "lobby" and "nice one" in t1["n"]["b"], t1)
    check("a second tagged person is notified too", True if (await nn(cleo, "tag"))["n"]["a"] == "ann" else False)
    await say(ann, "again @bob"); await recv(ann, "m")
    t2 = await nn(bob, "tag")
    check("more tags from the same person in the same room group into one row", t2["n"]["i"] == t1["n"]["i"] and t2["n"]["n"] == 2, t2)
    await say(ann, "@nobodyhere and @ann myself"); await recv(ann, "m")
    check("tagging someone who does not exist or yourself makes nothing", await silent(ann) and await silent(dan))
    await say(ann, "@bob @cleo @dan @boss four people"); await recv(ann, "m")
    got = [await nn(w, "tag", 2) for w in (bob, cleo, dan)]
    check("at most three people are notified per message", await silent(boss, "nn", 1.0), "boss got one")
    # reply: bob answers ann's message while ann is in another room
    await send(ann, t="join", r="abuja"); await recv(ann, "joined"); await drain(ann, 0.3)
    await send(bob, t="join", r="lobby"); await recv(bob, "joined"); await drain(bob, 0.3)
    await send(bob, t="m", b="thanks for that", q=ma["i"]); await recv(bob, "m")
    r1 = await nn(ann, "reply")
    check("a reply notifies the author of the message it answers", r1["n"]["a"] == "bob" and r1["n"]["rn"] == "General Chat", r1)
    await send(cleo, t="join", r="lobby"); await recv(cleo, "joined"); await drain(cleo, 0.3)
    await say(cleo, "hello again @bob"); await recv(cleo, "m")
    check("nobody is notified for a room they are looking at", await silent(bob, "nn", 1.0))

    # ---- gifts
    await send(bob, t="join", r="lobby"); await recv(bob, "joined"); await drain(bob, 0.3)
    await send(bob, t="m", b="gift me a pizza"); mb = await recv(bob, "m")
    await send(ann, t="join", r="lobby"); await recv(ann, "joined"); await drain(ann, 0.3)
    await send(bob, t="join", r="abuja"); await recv(bob, "joined"); await drain(bob, 0.3)
    await send(cleo, t="join", r="football"); await recv(cleo, "joined"); await drain(cleo, 0.3)
    await send(ann, t="tip", i=mb["i"], amt=5, c="notify-key-0001")
    g = await nn(bob, "tip")
    check("a gift notifies the receiver with the amount, the sender and the room", g["n"]["a"] == "ann" and g["n"]["m"] == 5 and g["n"]["rn"] == "General Chat" and g["n"]["b"], g)
    await send(ann, t="tip", i=mb["i"], amt=5, c="notify-key-0001")
    check("repeating the same gift request does not notify twice", await silent(bob, "nn", 1.0))
    await send(ann, t="tip", i=mb["i"], amt=5, c="notify-key-0002")
    g2 = await nn(bob, "tip"); check("a different gift is a new notification", g2["n"]["i"] != g["n"]["i"])

    # ---- friend requests
    await send(cleo, t="fr", a="add", h="bob"); await recv(cleo, "frok")
    f1 = await nn(bob, "friend"); check("a friend request notifies the other person", f1["n"]["a"] == "cleo" and f1["n"]["b"] == "request", f1)
    await send(bob, t="fr", a="accept", h="cleo"); await recv(bob, "frok")
    f2 = await nn(cleo, "friend"); check("accepting notifies the requester", f2["n"]["a"] == "bob" and f2["n"]["b"] == "accepted", f2)

    # ---- reading, and only my own
    lst, c = await nl(bob)
    check("everything above is in bob's list, newest first", len(lst) >= 5 and lst == sorted(lst, key=lambda x: -x["ts"]) and c == sum(1 for x in lst if x.get("u")), (c, len(lst)))
    one = next(x for x in lst if x.get("u"))
    await send(bob, t="nread", i=one["i"]); c2 = await count(bob)
    check("reading one lowers the count by one", c2 == c - 1, (c, c2))
    cleo_lst, cleo_c = await nl(cleo)
    cleo_ids = {x["i"] for x in cleo_lst}
    await send(ann, t="nread", i=one["i"]); await count(ann)
    lst2, c3 = await nl(bob)
    check("someone else cannot mark my notification read", c3 == c2 and any(x["i"] == one["i"] and "u" not in x for x in lst2) )
    check("nobody sees another person's list", not ({x["i"] for x in lst} & cleo_ids) , (cleo_ids))
    await send(bob, t="nread", i=999999); c4 = await count(bob); check("an unknown id changes nothing", c4 == c3)
    await send(bob, t="nread"); e = await recv(bob, "err"); check("a request with no id and no all flag is refused", e["c"] == "bad_request", e)
    await send(bob, t="nread", all=True); check("mark all read empties the count", await count(bob) == 0)
    lst, c = await nl(bob); check("and nothing is unread in the list", c == 0 and not any(x.get("u") for x in lst))

    # ---- settings
    check("settings are saved and echoed", (await set_pref(bob, "nk", "0"))["nk"] is False)
    await say(ann, "tag again @bob"); await recv(ann, "m")
    check("a switched off kind makes no notification", await silent(bob))
    await set_pref(bob, "nk", "1")
    await set_pref(bob, "nd", "0")
    await send(ann, t="dm", h="bob", b="quiet please"); await recv(ann, "dm")
    check("direct messages can be switched off, the message still arrives", await silent(bob) and True)
    inbox_before = None
    await send(bob, t="inbox"); ib = await recv(bob, "inbox")
    check("the message is still in the inbox", any(c["h"] == "ann" for c in ib["c"]))
    await set_pref(bob, "nd", "1"); await set_pref(bob, "nt", "0"); await set_pref(bob, "nf", "0")
    check("friend and gift notifications can be switched off", (await set_pref(bob, "nt", "0"))["nt"] is False)
    for bad in (dict(k="zz", b="1"), dict(k="nd", b="maybe"), dict(k="dp", b="nobody"), dict(k="so", b="yes"), dict(k="", b="1"), dict(k="n_dm", b="0")):
        await send(bob, t="set", **bad); e = await recv(bob, "err"); check("an invalid setting is refused: %s" % bad, e["c"] == "bad_request", e)
    await bob.close(); await asyncio.sleep(0.3)
    bob = await join(token=okb["k"]); _, pf, _ = await ready_n(bob)
    check("settings are still there after signing in again", pf["nt"] is False and pf["nf"] is False and pf["nd"] and pf["nk"], pf)
    row = sql("SELECT n_tip, n_friend FROM user_prefs WHERE user_id = (SELECT id FROM users WHERE handle_key = 'bob')")
    check("they are in the database", row == "f|f", row)
    await set_pref(bob, "nt", "1"); await set_pref(bob, "nf", "1")

    # ---- privacy: who may message me
    pf = await set_pref(bob, "dp", "friends"); check("friends only is saved", pf["dp"] == "friends")
    await send(dan, t="dm", h="bob", b="hi from a stranger"); e = await recv(dan, "err"); check("a stranger is refused when bob takes friends only", e["c"] == "dm_closed", e)
    await send(dan, t="thread", h="bob"); th = await recv(dan, "thread"); check("the conversation tells the stranger why", th.get("cl") is True, th)
    await send(cleo, t="dm", h="bob", b="friend message"); m = await recv(cleo, "dm"); check("a friend can still message", m["b"] == "friend message")
    await send(boss, t="dm", h="bob", b="staff message"); m = await recv(boss, "dm"); check("staff can still reach them", m["b"] == "staff message")
    await send(cleo, t="thread", h="bob"); th = await recv(cleo, "thread"); check("friends see no warning", not th.get("cl"))
    await set_pref(bob, "dp", "everyone")
    await send(dan, t="dm", h="bob", b="hi again"); m = await recv(dan, "dm"); check("switching back to everyone reopens it", m["b"] == "hi again")
    # hide online
    await send(cleo, t="profile", h="bob"); p = await recv(cleo, "uprofile"); check("bob shows as online by default", p["on"] is True)
    pf = await set_pref(bob, "so", "0"); check("hiding is saved", pf["so"] is False)
    await send(dan, t="profile", h="bob"); p = await recv(dan, "uprofile"); check("others no longer see bob online", p["on"] is False, p)
    await send(boss, t="profile", h="bob"); p = await recv(boss, "uprofile"); check("staff still do", p["on"] is True)
    await send(bob, t="profile", h="bob"); p = await recv(bob, "uprofile"); check("and he sees himself as online", p["on"] is True)
    await set_pref(bob, "so", "1")
    # blocking
    await send(bob, t="blk", a="add", h="dan"); await recv(bob, "blkok")
    await send(dan, t="dm", h="bob", b="blocked msg"); e = await recv(dan, "err"); check("a blocked person cannot message", e["c"] == "blocked", e)
    await send(dan, t="join", r="lobby"); await recv(dan, "joined"); await send(bob, t="join", r="abuja"); await recv(bob, "joined"); await drain(bob, 0.3)
    await say(dan, "hey @bob it is me"); await recv(dan, "m")
    check("a blocked person's tags make no notification", await silent(bob, "nn", 1.0))

    # ---- change password
    async def pwc(ws, o, p):
        await send(ws, t="pwchange", o=o, p=p)
        while True:
            m = await recv(ws, None)
            if m["t"] in ("err", "pwchanged"): return m
    await drain(ann, 0.4); await drain(dan, 0.4)
    m = await pwc(ann, "wrong password", "new password 12"); check("a wrong current password is refused", m["t"] == "err" and m["c"] == "bad_login", m)
    m = await pwc(ann, "ann password 1", "short"); check("a weak new password is refused", m["t"] == "err" and m["c"] == "weak_password", m)
    m = await pwc(dan, "x", "new password 12"); check("an account with no password cannot use it", m["t"] == "err" and m["c"] == "needs_password", m)
    other = await join(login=("ann", "ann password 1")); okx = await ready(other)
    m = await pwc(ann, "ann password 1", "new password 12"); check("the right current password changes it", m["t"] == "pwchanged", m)
    check("this login keeps working", (await nl(ann))[1] >= 0)
    chk = await join(token=okx["k"]); m = await recv(chk, None); check("its saved token no longer signs in", m["t"] == "err", m)
    old = await join(login=("ann", "ann password 1")); m = await recv(old, None); check("the old password no longer works", m["t"] == "err" and m["c"] == "bad_login", m)
    new = await join(login=("ann", "new password 12")); m = await recv(new, "ok"); check("the new password works", m["h"] == "ann")
    for i in range(9):
        r = await pwc(ann, "guess %d" % i, "another one 123")
    check("guessing is rate limited like logging in", r["t"] == "err" and r["c"] == "too_many_attempts", r)

    stop()
    print(f"\n{passed} passed, {failed} failed")
    raise SystemExit(1 if failed else 0)

asyncio.run(main())
