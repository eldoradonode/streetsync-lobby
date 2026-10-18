import asyncio, json, os, signal, subprocess, time, urllib.request, websockets

PORT = 8100
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

def start(**extra):
    global proc
    proc = subprocess.Popen([BIN], env=dict(BASE, **extra), stdout=open("/tmp/lobby_rooms2.log", "a"), stderr=subprocess.STDOUT)
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

async def err(ws, timeout=2):
    m = await recv(ws, "err", timeout)
    return m["c"] if m.get("m") is None else (m["c"], m["m"])

async def mk(handle, points=0, grant=0):
    """A signed-in person. points set their tier, grant adds credits through the ledger."""
    ws = await conn(handle); ok = await recv(ws, "ok"); await me_with(ws, 20); tok = ok["k"]
    await ws.close(); await asyncio.sleep(1.4)
    key = handle.lower()
    sql(f"UPDATE users SET points={points} WHERE handle_key='{key}'")
    if grant:
        sql(f"INSERT INTO credit_ledger (user_id, kind, amount, balance_after, ref, note) SELECT id, 'grant', {grant}, credits+{grant}, 'e2e-{key}', '' FROM users WHERE handle_key='{key}'")
        sql(f"UPDATE users SET credits=credits+{grant} WHERE handle_key='{key}'")
    ws = await conn(token=tok); await recv(ws, "ok")
    return ws, tok

def bal(handle): return int(sql(f"SELECT credits FROM users WHERE handle_key='{handle.lower()}'"))

async def rooms_where(ws, pred, timeout=3):
    """The first room list that satisfies pred, or the last one seen."""
    end = time.time() + timeout; last = None
    while time.time() < end:
        try: m = json.loads(await asyncio.wait_for(ws.recv(), max(0.05, end - time.time())))
        except asyncio.TimeoutError: break
        if m["t"] == "rooms":
            last = m
            if pred(m["r"]): return m["r"]
    return last["r"] if last else None

def find_room(rooms, slug): return next((r for r in rooms or [] if r["s"] == slug), None)

LAST_ME = {}

async def try_join(ws, **kw):
    """Join and return the joined or err frame. Balance updates that arrive on the way are remembered in LAST_ME."""
    await send(ws, t="join", **kw)
    while True:
        m = await recv(ws, None, 3)
        if m["t"] == "me": LAST_ME[ws] = m
        if m["t"] in ("joined", "err"): return m

async def leave(ws):
    await send(ws, t="leave"); await recv(ws, "left")

async def say(ws, body):
    await send(ws, t="m", b=body)

async def main():
    sql("TRUNCATE users, sessions, messages, sanctions, reports, mod_log, rooms RESTART IDENTITY CASCADE")
    start()
    boss = await conn(login=("Boss", "boss password 1")); await recv(boss, "ok")
    hana, thana = await mk("hana", 300, 100)
    ivy, tivy = await mk("ivy", 0, 0)            # a Newbie
    kim, tkim = await mk("kim", 300, 0)          # Regular, only the welcome credits
    jon, tjon = await mk("jon", 300, 100)
    ned, tned = await mk("ned", 300, 0)

    # ---- who can create, and what is refused
    await send(ivy, t="croom", n="Ivy Place", k="public"); check("a Newbie can not create a room", await err(ivy) == ("tier_locked", 1))
    await send(hana, t="croom", n="x", k="public"); check("a too short name", await err(hana) == "bad_room_name")
    await send(hana, t="croom", n="<b>bold</b>", k="public"); check("a name with markup", await err(hana) == "bad_room_name")
    await send(hana, t="croom", n="Good Name", k="secret"); check("an unknown kind", await err(hana) == "bad_room_kind")
    await send(hana, t="croom", n="Good Name", k="public", f=51); check("a fee above the limit", await err(hana) == "bad_fee")
    await send(hana, t="croom", n="Good Name", k="public", f=-1); check("a negative fee", await err(hana) == "bad_fee")
    await send(hana, t="croom", n="Good Name", k="public", p="abc"); check("a too short password", await err(hana) == "bad_room_password")
    await send(kim, t="croom", n="Kim Place", k="public"); check("not enough credits to create", await err(kim) == ("no_credits", 25))
    check("nothing was created or charged by the refusals", sql("SELECT count(*) FROM rooms WHERE kind <> 'official'") == "0" and bal("kim") == 20 and bal("hana") == 120)

    # ---- creating a public room
    await send(hana, t="croom", n="Abuja Night Owls", k="public", g="Gist, Night, Abuja, Extra")
    ok = await recv(hana, "croomok")
    slug, invite = ok["s"], ok["i"]
    check("creating answers with the address and an invite code", slug.startswith("abuja-night-owls-") and len(invite) == 8 and ok["n"] == "Abuja Night Owls", ok)
    m = await me_with(hana, 95)
    check("it costs 25 credits", m is not None and m["c"] == 95 and bal("hana") == 95, m)
    check("the ledger says why", sql("SELECT note FROM credit_ledger WHERE kind='room_create'") == "Created the room Abuja Night Owls")
    check("tags are cut to three", sql(f"SELECT tags FROM rooms WHERE slug='{slug}'") == "Gist,Night,Abuja")
    rooms = await rooms_where(hana, lambda r: find_room(r, slug) is not None)
    r = find_room(rooms, slug)
    check("the host sees their own room even though it is empty", r is not None and r["u"] == 0 and r["k"] == "public" and r["o"] == "hana" and r["y"] is True and "f" not in r and "w" not in r, r)
    check("official rooms are still all there", len([x for x in rooms if "k" not in x]) == 10)
    await send(hana, t="croom", n="Second Room", k="public"); check("one room per person", await err(hana) == ("room_limit", 1))
    check("and it did not charge", bal("hana") == 95)
    await send(jon, t="croom", n="abuja night OWLS", k="public"); check("names are unique whatever the capitals", await err(jon) == "room_name_taken")
    await send(jon, t="croom", n="Sport", k="public"); check("official names are taken too", await err(jon) == "room_name_taken")
    check("jon was not charged for either", bal("jon") == 120)

    # ---- listing: only while people are in it
    rooms = await rooms_where(ivy, lambda r: True, 1.8)
    check("an empty community room is not on the hub for others", find_room(rooms, slug) is None)
    r = await try_join(hana, r=slug)
    check("the host joins and learns the invite code", r["t"] == "joined" and r["k"] == "public" and r["o"] == "hana" and r["y"] is True and r["i"] == invite, r)
    rooms = await rooms_where(ivy, lambda r: find_room(r, slug) is not None, 3)
    r = find_room(rooms, slug)
    check("once someone is inside it appears on the hub for everyone", r is not None and r["u"] == 1 and r["o"] == "hana" and r.get("y") is not True, r)
    r = await try_join(ivy, r=slug)
    check("anyone can enter a free public room, even a Newbie", r["t"] == "joined" and "i" not in r and "y" not in r, r)
    await say(ivy, "hello owls")
    m = await recv(hana, "m")
    check("messages work as in any room", m["b"] == "hello owls" and m["h"] == "ivy", m)
    await leave(ivy)

    # ---- searching
    await send(ivy, t="find", n="owls"); f = await recv(ivy, "found")
    check("search finds a public room by name", [x["s"] for x in f["r"]] == [slug], f)
    await send(ivy, t="find", n="night"); f = await recv(ivy, "found")
    check("and by tag", [x["s"] for x in f["r"]] == [slug], f)
    await send(ivy, t="find", n="zzzz"); f = await recv(ivy, "found")
    check("nothing matches nonsense", f["r"] == [], f)
    await send(ivy, t="find", n="a" * 40); check("a very long query is refused", await err(ivy) == "bad_query")
    await leave(hana)
    rooms = await rooms_where(ivy, lambda r: find_room(r, slug) is None, 3)
    check("when everyone leaves the room leaves the hub", find_room(rooms, slug) is None)
    await send(ivy, t="find", n="owls"); f = await recv(ivy, "found")
    check("but search still finds it, empty", [x["s"] for x in f["r"]] == [slug] and f["r"][0]["u"] == 0, f)
    rooms = await rooms_where(hana, lambda r: find_room(r, slug) is not None, 1.8)
    check("and the host still sees it", find_room(rooms, slug) is not None)

    # ---- a private room is entered by invite code
    await send(jon, t="croom", n="Secret Club", k="private"); ok = await recv(jon, "croomok")
    sslug, sinv = ok["s"], ok["i"]
    await send(ivy, t="find", n="secret"); f = await recv(ivy, "found")
    check("private rooms are never found by search", f["r"] == [], f)
    r = await try_join(jon, r=sslug)
    rooms = await rooms_where(ivy, lambda r: True, 1.8)
    check("an occupied private room is still not on the hub", find_room(rooms, sslug) is None)
    r = await try_join(ivy, r=sslug); check("joining a private room without the code", r["t"] == "err" and r["c"] == "needs_invite", r)
    r = await try_join(ivy, r=sslug, c="AAAAAAAA"); check("with a wrong code", r["t"] == "err" and r["c"] == "needs_invite", r)
    await send(ivy, t="rinfo", c="AAAAAAAA"); check("looking up a wrong code", await err(ivy) == "no_such_room")
    await send(ivy, t="rinfo", c=sinv.lower()); ri = await recv(ivy, "rinfo")
    check("a right code, in any case, tells the name, host and price", ri["s"] == sslug and ri["n"] == "Secret Club" and ri["o"] == "jon" and ri["f"] == 0 and ri["w"] is False and ri["y"] is False, ri)
    r = await try_join(ivy, r=sslug, c=sinv); check("the code lets her in", r["t"] == "joined" and "i" not in r, r)
    check("she is now a member", sql(f"SELECT count(*) FROM room_members WHERE room='{sslug}'") == "1")
    await leave(ivy)
    r = await try_join(ivy, r=sslug); check("a member comes back without the code", r["t"] == "joined", r)
    await leave(ivy)
    r = await try_join(kim, r=sslug); check("a stranger still can not", r["t"] == "err" and r["c"] == "needs_invite", r)
    rooms = await rooms_where(ivy, lambda r: find_room(r, sslug) is not None, 3)
    check("a member sees the private room in their list", find_room(rooms, sslug) is not None and find_room(rooms, sslug)["y"] is True)
    await leave(jon)

    # ---- a password room
    lee, tlee = await mk("lee", 300, 100)
    await send(lee, t="croom", n="Pass Room", k="public", p="open sesame"); ok = await recv(lee, "croomok")
    pslug, pinv = ok["s"], ok["i"]
    check("the password is stored hashed", sql(f"SELECT pass_hash LIKE '%open sesame%' OR pass_hash IS NULL FROM rooms WHERE slug='{pslug}'") == "f")
    await try_join(lee, r=pslug)
    rooms = await rooms_where(kim, lambda r: True, 1.8)
    check("a room with a password is never on the hub, even occupied", find_room(rooms, pslug) is None)
    await send(kim, t="find", n="pass"); f = await recv(kim, "found")
    check("or in search", f["r"] == [], f)
    r = await try_join(kim, r=pslug); check("no code", r["c"] == "needs_invite", r)
    r = await try_join(kim, r=pslug, c=pinv); check("code but no password", r["c"] == "bad_room_password", r)
    r = await try_join(kim, r=pslug, c=pinv, p="Open Sesame"); check("a password in the wrong case", r["c"] == "bad_room_password", r)
    r = await try_join(kim, r=pslug, c=pinv, p="open sesame"); check("code and password open it", r["t"] == "joined" and r["w"] is True, r)
    await leave(kim)
    r = await try_join(kim, r=pslug); check("and next time neither is needed", r["t"] == "joined", r)
    await leave(kim)
    for i in range(5):
        r = await try_join(ned, r=pslug, c=pinv, p="wrong" + str(i))
    r = await try_join(ned, r=pslug, c=pinv, p="open sesame")
    check("five wrong passwords lock the attempts, even for the right one", r["t"] == "err" and r["c"] == "too_many_attempts", r)

    # ---- an entry fee
    await try_join(hana, r=slug)
    await send(hana, t="rset", f=10); rs = await recv(hana, "rsetok")
    check("the host sets a fee", rs["f"] == 10 and rs["w"] is False and rs["k"] == "public", rs)
    r = await try_join(jon, r=slug); check("joining without agreeing to the price", r["t"] == "err" and r["c"] == "fee_changed" and r["m"] == 10, r)
    r = await try_join(jon, r=slug, f=5); check("agreeing to a different price", r["c"] == "fee_changed" and r["m"] == 10, r)
    check("nothing was charged", bal("jon") == 95)
    r = await try_join(jon, r=slug, f=10)
    check("agreeing to the real price lets him in", r["t"] == "joined" and r["f"] == 10, r)
    check("he paid the fee, and his page was told", bal("jon") == 85 and LAST_ME.get(jon, {}).get("c") == 85, LAST_ME.get(jon))
    m = await me_with(hana, 103)
    check("the host got 80 percent at once", m is not None and m["c"] == 103 and bal("hana") == 103, m)
    check("both sides are in the ledger", sql("SELECT string_agg(kind || ':' || amount, ',' ORDER BY id) FROM credit_ledger WHERE kind IN ('room_entry','room_income')") == "room_entry:-10,room_income:8")
    await leave(jon)
    await send(hana, t="rset", f=50); await recv(hana, "rsetok")
    r = await try_join(jon, r=slug); check("a member comes back free even after a price rise", r["t"] == "joined", r)
    check("and is not charged again", bal("jon") == 85 and bal("hana") == 103)
    r = await try_join(kim, r=slug, f=50); check("not enough credits for the fee", r["c"] == "no_credits" and r["m"] == 50, r)
    check("and no membership came of it", sql(f"SELECT count(*) FROM room_members WHERE room='{slug}' AND user_id=(SELECT id FROM users WHERE handle_key='kim')") == "0" and bal("kim") == 20)
    rooms = await rooms_where(ivy, lambda r: find_room(r, slug) and find_room(r, slug).get("f") == 50, 3)
    check("the hub shows the price", find_room(rooms, slug) is not None and find_room(rooms, slug)["f"] == 50, find_room(rooms or [], slug))
    await send(hana, t="rset", f=0); await recv(hana, "rsetok")

    # ---- host settings are for the host
    await send(jon, t="rset", f=1); check("a member can not change settings", await err(jon) == "not_host")
    await send(hana, t="rset", f=99); check("a bad fee in settings", await err(hana) == "bad_fee")
    await send(hana, t="rset", k="secret"); check("a bad kind in settings", await err(hana) == "bad_room_kind")
    await send(hana, t="rset", pw="abc"); check("a bad password in settings", await err(hana) == "bad_room_password")

    # ---- the host as moderator of their own room
    await say(jon, "hello from jon"); mj = await recv(hana, "m")
    await send(hana, t="hmod", a="mute", h="jon", m=5)
    await recv(hana, "modok"); rm = await recv(jon, "rmuted")
    check("the host can mute a member in the room", rm["m"] == 5, rm)
    await say(jon, "can i talk"); check("a muted member can not post there", await err(jon) == "room_muted")
    await send(jon, t="leave"); await recv(jon, "left")
    r = await try_join(jon, r="lobby"); await say(jon, "i can talk elsewhere")
    mm = await recv(jon, "m")
    check("but is not muted anywhere else", mm["b"] == "i can talk elsewhere", mm)
    await leave(jon); r = await try_join(jon, r=slug)
    await send(hana, t="hmod", a="unmute", h="jon"); await recv(hana, "modok")
    await say(jon, "unmuted now"); mm = await recv(jon, "m")
    check("unmuting works", mm["b"] == "unmuted now")
    await send(hana, t="hmod", a="mute", h="Boss", m=5); check("the host can not mute staff", await err(hana) == "forbidden")
    await send(hana, t="hmod", a="mute", h="hana", m=5); check("or themselves", await err(hana) == "forbidden")
    await send(jon, t="hmod", a="mute", h="hana", m=5); check("a member has no host powers", await err(jon) == "not_host")
    await send(hana, t="hmod", a="dance", h="jon"); check("an unknown host action", await err(hana) == "bad_action")
    # deleting
    await say(jon, "delete me please"); md = await recv(hana, "m")
    await send(hana, t="del", i=md["i"]); dl = await recv(hana, "del")
    check("the host can delete a member's message", dl["i"] == md["i"], dl)
    await say(hana, "host speaking"); mh = await recv(hana, "m")
    await send(jon, t="del", i=mh["i"]); e = await err(jon)
    check("a member can not delete other people's messages", e in ("forbidden", "msg_gone"), e)
    bs = await try_join(boss, r=slug); await say(boss, "staff speaking"); ms = await recv(hana, "m")
    await send(hana, t="del", i=ms["i"]); check("the host can not delete a staff message", await err(hana) == "forbidden")
    # kicking
    await send(hana, t="hmod", a="kick", h="jon", m=30)
    await recv(hana, "modok"); rk = await recv(jon, "rkicked")
    check("kicked members are told, and sent to the hub", rk["m"] == 30 and rk["n"] == "Abuja Night Owls", rk)
    await say(jon, "still here?"); check("a kicked member is out of the room", await err(jon) == "no_room")
    r = await try_join(jon, r=slug); check("and can not come back for a while", r["t"] == "err" and r["c"] == "room_banned" and r["m"] in (29, 30), r)
    check("the ban is only in that room", (await try_join(jon, r="lobby"))["t"] == "joined")
    await leave(jon)

    # ---- closing
    await send(ned, t="rclose"); check("a member can not close a room", await err(ned) == "not_host")
    await try_join(ivy, r=slug)
    await send(hana, t="rclose"); await recv(hana, "modok")
    rc = await recv(ivy, "rclosed")
    check("people inside are told the room closed", rc["s"] == slug and rc["n"] == "Abuja Night Owls", rc)
    check("the room, its members and its messages are gone", sql(f"SELECT count(*) FROM rooms WHERE slug='{slug}'") == "0" and sql(f"SELECT count(*) FROM messages WHERE room='{slug}'") == "0")
    rooms = await rooms_where(hana, lambda r: find_room(r, slug) is None, 3)
    check("it is gone from the host's list", find_room(rooms, slug) is None)
    r = await try_join(ivy, r=slug); check("and can not be entered", r["c"] == "no_such_room", r)
    await send(hana, t="croom", n="Abuja Night Owls", k="public"); ok = await recv(hana, "croomok")
    check("the host can create again, paying 25 again (balance 103, plus 2 daily credits for chatting, minus 25)", bal("hana") == 103 + 2 - 25, bal("hana"))
    # staff can close anyone's room
    await send(boss, t="rclose", r=sslug); await recv(boss, "modok")
    check("staff can close any community room", sql(f"SELECT count(*) FROM rooms WHERE slug='{sslug}'") == "0")
    check("and it is in the mod log", sql("SELECT count(*) FROM mod_log WHERE action='close_room'") == "1")
    await send(boss, t="rclose", r="lobby"); e = await err(boss)
    check("official rooms can not be closed this way", e == "not_host", e)

    # ---- everything survives a restart, and idle rooms close
    await asyncio.sleep(0.3)
    for ws in (boss, hana, ivy, kim, jon, ned, lee): await ws.close()
    await asyncio.sleep(1.0)
    sql(f"UPDATE rooms SET last_active = now() - interval '4 days' WHERE slug='{pslug}'")
    newslug = sql("SELECT slug FROM rooms WHERE name='Abuja Night Owls'")
    stop(); start(FIRST_CLEANUP_SECONDS="2")
    hana = await conn(token=thana); await recv(hana, "ok")
    rooms = await rooms_where(hana, lambda r: find_room(r, newslug) is not None, 3)
    check("rooms and ownership survive a restart", find_room(rooms, newslug) is not None and find_room(rooms, newslug)["y"] is True)
    await asyncio.sleep(3.0)
    check("a room unused for 4 days is closed by the cleanup", sql(f"SELECT count(*) FROM rooms WHERE slug='{pslug}'") == "0")
    check("a fresh room is kept", sql(f"SELECT count(*) FROM rooms WHERE slug='{newslug}'") == "1")
    off = sql("SELECT count(*) FROM users u WHERE credits <> coalesce((SELECT sum(amount) FROM credit_ledger l WHERE l.user_id=u.id),0)")
    check("every balance equals its ledger", off == "0", off)
    await hana.close()
    stop()
    print(f"\n{passed} passed, {failed} failed")
    raise SystemExit(1 if failed else 0)

asyncio.run(main())
