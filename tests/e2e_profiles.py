import asyncio, json, os, signal, subprocess, time, urllib.request, websockets

PORT = 8102
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
    proc = subprocess.Popen([BIN], env=BASE, stdout=open("/tmp/lobby_profiles.log", "a"), stderr=subprocess.STDOUT)
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

async def conn(handle=None, token=None, login=None, room=None, last=0, look=None):
    ws = await websockets.connect(URL)
    extra = {"r": room, "l": last} if room else {}
    if look is not None: extra["v"] = look
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


def send(ws, **kw): return ws.send(json.dumps(kw))
async def err(ws, timeout=2): return (await recv(ws, "err", timeout))["c"]

async def collect(ws, wait=0.8):
    out = []; end = time.time() + wait
    while time.time() < end:
        try: out.append(json.loads(await asyncio.wait_for(ws.recv(), max(0.05, end - time.time()))))
        except asyncio.TimeoutError: break
    return out

def first(frames, t, **want):
    return next((f for f in frames if f["t"] == t and all(f.get(k) == v for k, v in want.items())), None)

async def main():
    sql("TRUNCATE users, sessions, messages, sanctions, reports, mod_log, rooms RESTART IDENTITY CASCADE")
    start()

    # ---- picking a look at sign up
    ann = await conn("ann", look="f3"); ok = await recv(ann, "ok")
    check("a look picked at sign up comes back on the ok frame", ok["v"] == "f3", ok)
    bob = await conn("bob", look="<script>"); okb = await recv(bob, "ok")
    check("a made up look is dropped", "v" not in okb, okb)
    cat = await conn("cat"); okc = await recv(cat, "ok")
    check("no pick means no look", "v" not in okc, okc)
    dan = await conn("dan", look="m8"); okd = await recv(dan, "ok")
    check("a number outside the set is dropped", "v" not in okd, okd)
    check("the look is stored", sql("SELECT avatar FROM users WHERE handle_key='ann'") == "f3" and sql("SELECT avatar FROM users WHERE handle_key='bob'") == "")

    # ---- changing it
    await send(bob, t="av", c="m2"); f = await collect(bob)
    check("changing the look sends a me frame with it", first(f, "me", v="m2") is not None, [x for x in f if x["t"] == "me"])
    check("and stores it", sql("SELECT avatar FROM users WHERE handle_key='bob'") == "m2")
    for bad in ("", "q1", "m9", "f", "<b>", "m2 "):
        await send(bob, t="av", c=bad); e = await err(bob)
        check(f"a bad look {bad!r} is refused", e == "bad_avatar", e)
    check("nothing changed after the refusals", sql("SELECT avatar FROM users WHERE handle_key='bob'") == "m2")
    bob2 = await conn(token=okb["k"]); okb2 = await recv(bob2, "ok")
    check("another tab gets the look on sign in", okb2["v"] == "m2", okb2)

    # ---- messages carry the look, and so does history
    for ws in (ann, bob, cat): await send(ws, t="join", r="lobby"); await recv(ws, "joined"); await collect(ws, 0.3)
    await send(ann, t="m", b="hello from ann"); await send(cat, t="m", b="hello from cat")
    fb = await collect(bob)
    check("a message shows the look of the sender", first(fb, "m", h="ann", av="f3") is not None, [x for x in fb if x["t"] == "m"])
    check("and none for someone who never picked", (first(fb, "m", h="cat") or {}).get("av") is None and first(fb, "m", h="cat") is not None)
    check("the look is saved with the message", sql("SELECT avatar FROM messages WHERE handle='ann'") == "f3")

    # ---- direct messages and the inbox
    await send(ann, t="dm", h="bob", b="hi bob")
    fa, fb = await collect(ann), await collect(bob)
    check("a direct message carries the sender's look", first(fb, "dm", f="ann", fa="f3") is not None, [x for x in fb if x["t"] == "dm"])
    await send(bob, t="thread", h="ann"); th = await recv(bob, "thread")
    check("a conversation carries both looks", th["av"] == {"ann": "f3", "bob": "m2"}, th.get("av"))
    await send(bob, t="inbox"); ib = await recv(bob, "inbox")
    check("the inbox carries the looks of everyone in it", ib["av"] == {"ann": "f3"}, ib.get("av"))
    await send(cat, t="inbox"); ic = await recv(cat, "inbox")
    check("and nothing when nobody has one", "av" not in ic or ic["av"] == {}, ic.get("av"))

    # ---- profiles
    await send(bob, t="profile", h="Ann"); p = await recv(bob, "uprofile")
    check("a profile shows the person, look, tier and points", p["h"] == "ann" and p["v"] == "f3" and p["r"] == 0 and p["p"] >= 1 and p["on"] is True and p["j"] > 1_700_000_000_000, p)
    check("and how I stand with them", p["s"] == "none", p)
    await send(ann, t="fr", a="add", h="bob"); await recv(ann, "frok")
    await send(bob, t="profile", h="ann"); p = await recv(bob, "uprofile")
    check("a request to me shows as got", p["s"] == "got", p)
    await send(bob, t="fr", a="accept", h="ann"); await recv(bob, "frok")
    await send(bob, t="profile", h="ann"); p = await recv(bob, "uprofile")
    check("friends show as friend", p["s"] == "friend", p)
    await send(bob, t="blk", a="add", h="ann"); await recv(bob, "blkok")
    await send(bob, t="profile", h="ann"); p = await recv(bob, "uprofile")
    check("someone I blocked shows as blocked", p["s"] == "blocked", p)
    await send(ann, t="profile", h="bob"); p = await recv(ann, "uprofile")
    check("but the person who was blocked is not told", p["s"] == "none", p)
    await send(bob, t="blk", a="del", h="ann"); await recv(bob, "blkok")
    await send(bob, t="profile", h="bob"); p = await recv(bob, "uprofile")
    check("my own profile has no relationship", "s" not in p, p)
    await send(bob, t="profile", h="nobody"); check("a missing person", await err(bob) == "no_such_user")
    await send(bob, t="profile", h="x"); check("a badly formed handle", await err(bob) == "no_such_user")
    await cat.close(); await asyncio.sleep(1.4)
    await send(bob, t="profile", h="cat"); p = await recv(bob, "uprofile")
    check("someone who left shows as offline", p["on"] is False and "v" not in p, p)
    boss = await conn(login=("Boss", "boss password 1")); await recv(boss, "ok")
    await send(bob, t="profile", h="Boss"); p = await recv(bob, "uprofile")
    check("staff show a badge instead of a tier and points", p["o"] == "admin" and "r" not in p and "p" not in p, p)

    # ---- the look and the Sport room survive a restart
    for ws in (ann, bob, bob2, dan, boss): await ws.close()
    await asyncio.sleep(1.2)
    check("the Sport room is an official room with sport tags", sql("SELECT name || '|' || tags FROM rooms WHERE slug='football'") == "Sport|Football,Basketball,Athletics", sql("SELECT name FROM rooms WHERE slug='football'"))
    sql("UPDATE rooms SET name='Football', tags='Matches,Banter,Transfers' WHERE slug='football'")
    stop(); start()
    check("an untouched Football room is renamed Sport at start", sql("SELECT name || '|' || tags FROM rooms WHERE slug='football'") == "Sport|Football,Basketball,Athletics", sql("SELECT name FROM rooms WHERE slug='football'"))
    sql("UPDATE rooms SET name='Football Banter' WHERE slug='football'")
    stop(); start()
    check("a room the admin renamed is left alone", sql("SELECT name FROM rooms WHERE slug='football'") == "Football Banter")
    ann = await conn(token=ok["k"], room="lobby"); okr = await recv(ann, "ok")
    check("the look is still there after a restart", okr["v"] == "f3", okr)
    f = await collect(ann, 1.0)
    hist = first(f, "hist")
    check("and in room history", hist and any(m.get("av") == "f3" for m in hist["m"]), hist)
    await ann.close(); stop()
    print(f"\n{passed} passed, {failed} failed")
    raise SystemExit(1 if failed else 0)

asyncio.run(main())
