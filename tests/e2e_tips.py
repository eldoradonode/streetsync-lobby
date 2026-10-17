import asyncio, json, os, signal, subprocess, time, urllib.request, websockets

PORT = 8101
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
    proc = subprocess.Popen([BIN], env=BASE, stdout=open("/tmp/lobby_tips.log", "a"), stderr=subprocess.STDOUT)
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

def send(ws, **kw): return ws.send(json.dumps(kw))
async def err(ws, timeout=2): return (await recv(ws, "err", timeout))["c"]

async def person(handle):
    ws = await conn(handle); ok = await recv(ws, "ok"); await me_with(ws, 20)
    return ws, ok["k"]

async def collect(ws, wait=1.0):
    """Every frame that arrives in the next moment, in order."""
    out = []; end = time.time() + wait
    while time.time() < end:
        try: out.append(json.loads(await asyncio.wait_for(ws.recv(), max(0.05, end - time.time()))))
        except asyncio.TimeoutError: break
    return out

def first(frames, t, **want):
    return next((f for f in frames if f["t"] == t and all(f.get(k) == v for k, v in want.items())), None)

async def in_room(ws, room="lobby"):
    await send(ws, t="join", r=room); await recv(ws, "joined"); await drain(ws, 0.2)

def bal(handle): return int(sql(f"SELECT credits FROM users WHERE handle_key='{handle.lower()}'"))

async def main():
    sql("TRUNCATE users, sessions, messages, sanctions, reports, mod_log, rooms RESTART IDENTITY CASCADE")
    start()
    ann, tann = await person("ann"); bob, tbob = await person("bob"); cat, tcat = await person("cat")
    for ws in (ann, bob, cat): await in_room(ws)
    await send(bob, t="m", b="great game last night"); mb = await recv(bob, "m"); await recv(ann, "m"); await recv(cat, "m")
    await send(ann, t="m", b="my own words"); ma = await recv(ann, "m"); await recv(bob, "m"); await recv(cat, "m")
    await asyncio.sleep(0.4)
    ann_before, bob_before = bal("ann"), bal("bob")

    # ---- a tip, end to end
    pts0 = sql("SELECT points FROM users WHERE handle_key='bob'")
    await send(ann, t="tip", i=mb["i"], amt=5, c="e2e-key-0001")
    fa, fb, fc = await collect(ann), await collect(bob), await collect(cat)
    ok = first(fa, "tipok")
    check("the sender is told it worked, with the treat and the person", ok and ok["a"] == 5 and ok["n"] == "Burger" and ok["h"] == "bob" and "dup" not in ok, ok)
    card = first(fb, "m", ta=5)
    check("everyone in the room sees a tip card", card and card["tt"] == "bob" and card["h"] == "ann" and card["b"] == "Burger" and first(fa, "m", ta=5) and first(fc, "m", ta=5)["i"] == card["i"], card)
    got = first(fb, "tipgot")
    check("the recipient is told who tipped them", got and got["f"] == "ann" and got["n"] == "Burger" and got["a"] == 5, got)
    check("the sender's balance went down by 5, live", first(fa, "me", c=ann_before - 5) is not None, [f for f in fa if f["t"] == "me"])
    check("the recipient's balance went up by 5, live", first(fb, "me", c=bob_before + 5) is not None, [f for f in fb if f["t"] == "me"])
    check("both balances are right in the database", bal("ann") == ann_before - 5 and bal("bob") == bob_before + 5)
    kinds = sql("SELECT string_agg(kind || ':' || amount || ':' || note, ' | ' ORDER BY id) FROM credit_ledger WHERE kind LIKE 'tip%'")
    check("the ledger has a sent and a received entry with names", kinds == "tip_sent:-5:Burger for @bob | tip_received:5:Burger from @ann", kinds)
    check("it is not a room fee", sql("SELECT count(*) FROM credit_ledger WHERE kind IN ('room_entry','room_income')") == "0")
    check("the card is stored with the tip details", sql("SELECT tip_to || ':' || tip_amount || ':' || body FROM messages WHERE tip_amount IS NOT NULL") == "bob:5:Burger")
    check("the person tipped earns no points from it", sql("SELECT points FROM users WHERE handle_key='bob'") == pts0)
    card = card

    # ---- double taps and retries
    await send(ann, t="tip", i=mb["i"], amt=5, c="e2e-key-0001")
    d = first(await collect(ann, 0.8), "tipok")
    check("repeating the same key is reported as already sent", d and d.get("dup") is True, d)
    for _ in range(3): await send(ann, t="tip", i=mb["i"], amt=10, c="e2e-key-double")
    r = [f for f in await collect(ann, 1.2) if f["t"] == "tipok"]
    check("three quick taps of one tip: one real, two repeats", sorted(bool(x.get("dup")) for x in r) == [False, True, True], r)
    await asyncio.sleep(0.4)
    check("so the sender paid 5 + 10 and no more", bal("ann") == ann_before - 15 and bal("bob") == bob_before + 15, (bal("ann"), bal("bob")))
    check("and there are exactly two tip cards", sql("SELECT count(*) FROM messages WHERE tip_amount IS NOT NULL") == "2")
    await send(cat, t="tip", i=mb["i"], amt=1, c="e2e-key-0001")
    ok = first(await collect(cat, 0.8), "tipok")
    check("another person using the same key still tips normally", not ok.get("dup") and bal("bob") == bob_before + 16, ok)

    # ---- refusals
    await send(ann, t="tip", i=ma["i"], amt=5, c="e2e-key-self1"); check("a tip to yourself", await err(ann) == "self_tip")
    for amt in (0, 2, 7, 100, -5):
        await send(ann, t="tip", i=mb["i"], amt=amt, c="e2e-key-amt" + str(abs(amt))); e = await err(ann)
        check(f"an amount of {amt} is refused", e == "bad_amount", e)
    for key in ("", "short", "bad key with spaces", "x" * 80):
        await send(ann, t="tip", i=mb["i"], amt=5, c=key); e = await err(ann)
        check(f"a bad key {key[:12]!r} is refused", e == "bad_request", e)
    await send(ann, t="tip", i=999999, amt=5, c="e2e-key-gone1"); check("a message that does not exist", await err(ann) == "msg_gone")
    await send(ann, t="tip", i=card["i"], amt=5, c="e2e-key-card1"); check("a tip card can not be tipped", await err(ann) == "forbidden")
    await send(ann, t="del", i=card["i"]); check("the sender can not delete their tip card", await err(ann) == "forbidden")
    await send(ann, t="m", b="replying", q=card["i"]); check("a tip card can not be replied to", await err(ann) == "reply_gone")
    await send(ann, t="leave"); await recv(ann, "left")
    await send(ann, t="tip", i=mb["i"], amt=5, c="e2e-key-noroom"); check("tips are sent from inside a room", await err(ann) == "no_room")
    await in_room(ann)
    check("none of the refusals moved credits", bal("ann") == ann_before - 15)

    # ---- not enough credits
    dan, tdan = await person("dan"); await in_room(dan)
    sql("INSERT INTO credit_ledger (user_id, kind, amount, balance_after, ref, note) SELECT id, 'spend', -17, 3, 'drain', '' FROM users WHERE handle_key='dan'")
    sql("UPDATE users SET credits=3 WHERE handle_key='dan'")
    await send(dan, t="tip", i=mb["i"], amt=5, c="e2e-key-poor1")
    check("a tip larger than the balance is refused", await err(dan) == "no_credits")
    check("and nothing moved", bal("dan") == 3 and sql("SELECT count(*) FROM credit_ledger WHERE user_id=(SELECT id FROM users WHERE handle_key='dan') AND kind LIKE 'tip%'") == "0")
    await send(dan, t="tip", i=mb["i"], amt=1, c="e2e-key-poor1")
    ok = first(await collect(dan, 0.8), "tipok")
    check("the same key works once the amount fits", not ok.get("dup") and bal("dan") == 2, ok)

    # ---- blocking stops tips
    await send(bob, t="blk", a="add", h="cat"); await recv(bob, "blkok")
    c0 = bal("cat")
    await send(cat, t="tip", i=mb["i"], amt=1, c="e2e-key-blk01"); check("a tip to someone who blocked you", await err(cat) == "blocked")
    check("and it cost nothing", bal("cat") == c0)

    # ---- the card survives a restart, and the books balance
    for ws in (ann, bob, cat, dan): await ws.close()
    await asyncio.sleep(1.2)
    stop(); start()
    ann = await conn(token=tann); await recv(ann, "ok")
    await send(ann, t="join", r="lobby"); await recv(ann, "joined"); h = await recv(ann, "hist")
    cards = [m for m in h["m"] if m.get("ta")]
    check("tip cards are in the room history after a restart", len(cards) == 4 and cards[0]["ta"] == 5 and cards[0]["tt"] == "bob" and cards[0]["b"] == "Burger", cards)
    off = sql("SELECT count(*) FROM users u WHERE credits <> coalesce((SELECT sum(amount) FROM credit_ledger l WHERE l.user_id=u.id),0)")
    check("every balance equals its ledger", off == "0", off)
    total = sql("SELECT sum(credits) FROM users"); gifts = sql("SELECT sum(amount) FROM credit_ledger WHERE kind IN ('welcome','daily','streak','grant')")
    check("tips made no credits and lost none", int(total) == int(gifts) - 17, (total, gifts))
    await ann.close(); stop()
    print(f"\n{passed} passed, {failed} failed")
    raise SystemExit(1 if failed else 0)

asyncio.run(main())
