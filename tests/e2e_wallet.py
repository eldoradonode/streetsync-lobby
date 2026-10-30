import asyncio, json, os, signal, subprocess, time, urllib.request, websockets

PORT = 8098
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
    proc = subprocess.Popen([BIN], env=BASE, stdout=open("/tmp/lobby_wallet.log", "a"), stderr=subprocess.STDOUT)
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
    """Wait for a me frame that shows this balance."""
    end = time.time() + timeout
    last = None
    while time.time() < end:
        try: m = json.loads(await asyncio.wait_for(ws.recv(), max(0.05, end - time.time())))
        except asyncio.TimeoutError: break
        if m["t"] == "me": last = m
        if m["t"] == "me" and m["c"] == credits: return m
    return last

async def wallet(ws):
    await ws.send(json.dumps({"t": "wallet"}))
    return await recv(ws, "wallet")

async def main():
    sql("TRUNCATE users, sessions, messages, sanctions, reports, mod_log, rooms RESTART IDENTITY CASCADE")
    start()

    # ---- welcome gift
    ann = await conn("ann")
    ok = await recv(ann, "ok")
    check("sign in tells the page a balance field", "c" in ok, ok)
    m = await me_with(ann, 20)
    check("a new account gets 20 welcome credits pushed to the page", m is not None and m["c"] == 20, m)
    w = await wallet(ann)
    check("wallet shows the balance and one welcome entry", w["c"] == 20 and len(w["l"]) == 1 and w["l"][0]["k"] == "welcome" and w["l"][0]["a"] == 20 and w["l"][0]["b"] == 20, w)
    check("a ledger entry carries a time", w["l"][0]["ts"] > 1_700_000_000_000, w["l"][0])
    token = ok["k"]

    # ---- reconnecting and extra tabs never pay again
    ann2 = await conn(token=token)
    ok2 = await recv(ann2, "ok")
    check("coming back shows the saved balance straight away", ok2["c"] == 20, ok2)
    ann3 = await conn(token=token)
    await recv(ann3, "ok")
    await asyncio.sleep(0.5)
    check("three tabs, one welcome gift", sql("SELECT count(*) FROM credit_ledger WHERE kind='welcome'") == "1")
    check("balance still 20", sql("SELECT credits FROM users WHERE handle_key='ann'") == "20")

    # ---- daily credits come from real chatting only
    await join(ann, "lobby"); await recv(ann, "joined")
    await drain(ann, 0.2)
    check("sitting online earns no daily credits", sql("SELECT count(*) FROM credit_ledger WHERE kind='daily'") == "0")
    await say(ann, "my first real message")
    m = await me_with(ann, 22)
    check("the first real message of the day pays 2 credits", m is not None and m["c"] == 22, m)
    m2 = await me_with(ann2, 22)
    check("another tab of the same person sees it too", m2 is not None and m2["c"] == 22, m2)
    await say(ann, "a second message today")
    await asyncio.sleep(0.5)
    check("a second message the same day pays no more credits", sql("SELECT credits FROM users WHERE handle_key='ann'") == "22")
    check("one daily entry in the ledger", sql("SELECT count(*) FROM credit_ledger WHERE kind='daily'") == "1")
    w = await wallet(ann)
    check("newest entry first", [e["k"] for e in w["l"]][:2] == ["daily", "welcome"] and w["c"] == 22, w)

    # ---- streak milestone: day 3 pays the daily credits plus 5
    bob = await conn("bob"); okb = await recv(bob, "ok"); await me_with(bob, 20)
    await bob.close()
    await asyncio.sleep(1.5)   # let the server forget bob, so it reads his streak from the database
    today = int(time.time() // 86400)
    sql(f"UPDATE users SET streak_days=2, streak_day={today - 1} WHERE handle_key='bob'")
    bob = await conn(token=okb["k"]); await recv(bob, "ok")
    await join(bob, "lobby"); await recv(bob, "joined")
    await say(bob, "day three streak")
    m = await me_with(bob, 27)
    check("day 3 of a streak pays daily 2 plus milestone 5", m is not None and m["c"] == 27 and m["s"] == 3, m)
    kinds = sql("SELECT string_agg(kind, ',' ORDER BY id) FROM credit_ledger WHERE user_id=(SELECT id FROM users WHERE handle_key='bob')")
    check("ledger shows welcome, daily and streak", kinds == "welcome,daily,streak", kinds)
    note = sql("SELECT note FROM credit_ledger WHERE kind='streak'")
    check("the streak entry says how long", note == "3 day streak", note)

    # ---- admin grants
    boss = await conn(login=("Boss", "boss password 1")); okboss = await recv(boss, "ok"); await drain(boss, 0.3)
    await boss.send(json.dumps({"t": "mod", "a": "grant", "h": "ann", "m": 50}))
    r = await recv(boss, "modok")
    check("admin grant is confirmed", r["a"] == "grant" and r["h"] == "ann" and r["m"] == 50, r)
    m = await me_with(ann, 72)
    check("the person sees the gift at once", m is not None and m["c"] == 72, m)
    w = await wallet(ann)
    check("the gift is in the ledger with a friendly note", w["l"][0]["k"] == "grant" and w["l"][0]["a"] == 50 and w["l"][0]["n"] == "Gift from the team", w["l"][0])
    check("the grant is in the mod log", sql("SELECT count(*) FROM mod_log WHERE action='grant' AND target='ann'") == "1")
    for amt in (0, -5, 100001):
        await boss.send(json.dumps({"t": "mod", "a": "grant", "h": "ann", "m": amt})); e = await recv(boss, "err")
        check(f"grant of {amt} is refused", e["c"] == "bad_amount", e)
    await boss.send(json.dumps({"t": "mod", "a": "grant", "h": "nobody", "m": 5})); e = await recv(boss, "err")
    check("grant to an unknown handle", e["c"] == "no_such_user", e)
    await ann.send(json.dumps({"t": "mod", "a": "grant", "h": "ann", "m": 999})); e = await recv(ann, "err")
    check("a normal member cannot grant credits", e["c"] == "forbidden", e)
    await asyncio.sleep(0.3)
    check("nothing changed after the refused grants", sql("SELECT credits FROM users WHERE handle_key='ann'") == "72")

    # ---- the books always balance, also after a restart
    for ws in (ann, ann2, ann3, bob, boss):
        await ws.close()
    await asyncio.sleep(1.0)
    stop(); start()
    ann = await conn(token=token); ok = await recv(ann, "ok")
    check("balance survives a server restart", ok["c"] == 72, ok)
    w = await wallet(ann)
    check("ledger survives a server restart", len(w["l"]) == 3 and w["c"] == 72, w)
    off = sql("SELECT count(*) FROM users u WHERE credits <> coalesce((SELECT sum(amount) FROM credit_ledger l WHERE l.user_id=u.id),0)")
    check("every balance equals the sum of its ledger", off == "0", off)
    check("the last balance on each user's ledger matches their balance", sql("SELECT count(*) FROM users u WHERE credits <> coalesce((SELECT balance_after FROM credit_ledger l WHERE l.user_id=u.id ORDER BY id DESC LIMIT 1),0)") == "0")
    await ann.close()
    stop()
    print(f"\n{passed} passed, {failed} failed")
    raise SystemExit(1 if failed else 0)

asyncio.run(main())
