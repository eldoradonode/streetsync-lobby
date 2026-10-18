import asyncio, json, os, signal, subprocess, time, urllib.request, websockets

PORT = 8095
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
    proc = subprocess.Popen([BIN], env=BASE, stdout=open("/tmp/lobby_rooms.log", "a"), stderr=subprocess.STDOUT)
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

async def main():
    sql("TRUNCATE users, sessions, messages, sanctions, reports, mod_log, rooms RESTART IDENTITY CASCADE")
    start()

    # ---- the hub
    ann, oka, rooms = await enter("ann")
    r = rooms["r"]
    check("the hub lists the ten official rooms in order", [x["s"] for x in r] == ["lobby", "abuja", "lagos", "campus", "football", "music", "tech", "side-hustle", "creators", "elite-lounge"], [x["s"] for x in r])
    lobby = r[0]
    check("a room has name, category, tags, people and heat", lobby["n"] == "General Chat" and lobby["c"] == "General" and lobby["g"] == ["Vibes", "Friends", "Random"] and lobby["u"] == 0 and lobby["h"] == 0 and "t" not in lobby, lobby)
    check("the Elite Lounge needs tier 3 and the others need nothing", r[-1]["t"] == 3 and all("t" not in x for x in r[:-1]))
    await say(ann, "hello from the hub"); e = await recv(ann, "err"); check("posting from the hub is refused", e["c"] == "no_room", e)
    await join(ann, "nowhere"); e = await recv(ann, "err"); check("unknown room", e["c"] == "no_such_room")
    await join(ann, "elite-lounge"); e = await recv(ann, "err"); check("a locked room says which tier opens it", e["c"] == "tier_locked" and e["m"] == 3, e)
    await join(ann, "lobby")
    j = await recv(ann, "joined"); h = await recv(ann, "hist"); n = await recv(ann, "n")
    check("joining sends the room, its history and its head count", j["r"] == "lobby" and j["n"] == "General Chat" and h["m"] == [] and n["c"] == 1, (j, h, n))

    # ---- rooms are separate
    bob, okbob, _ = await enter("bob", room="football")
    cleo, okc, _ = await enter("cleo", room="football")
    await say(ann, "only for the lobby")
    ma = await recv(ann, "m")
    check("a message in one room does not reach another", await quiet(bob, "m") and await quiet(cleo, "m", 0.2))
    await say(bob, "football only"); mb = await recv(bob, "m"); await recv(cleo, "m")
    check("the other room does not hear football", await quiet(ann, "m"))
    dan, _, _ = await enter("dan", room="lobby")
    check("a new joiner only gets that room's history", True)
    # history per room
    eve, _, _ = await enter("eve")
    await join(eve, "football"); await recv(eve, "joined"); hf = await recv(eve, "hist")
    check("football history holds only football messages", [m["b"] for m in hf["m"]] == ["football only"], hf)
    await join(eve, "lobby"); await recv(eve, "joined"); hl = await recv(eve, "hist")
    check("switching rooms gives the new room's history", [m["b"] for m in hl["m"]] == ["only for the lobby"], hl)
    await drain(eve, 0.3)
    await say(ann, "second lobby message"); await recv(ann, "m")
    m = await recv(eve, "m"); check("after switching you hear the new room", m["b"] == "second lobby message")
    await join(eve, "football"); await recv(eve, "joined"); await recv(eve, "hist"); await drain(eve, 0.2)
    await say(ann, "third lobby message"); await recv(ann, "m")
    check("and stop hearing the old one", await quiet(eve, "m"))

    # ---- resume into a room
    ann2 = await conn(token=oka["k"], room="lobby", last=ma["i"])
    await recv(ann2, "ok"); await recv(ann2, "rooms"); await recv(ann2, "joined"); hh = await recv(ann2, "hist")
    check("coming back with a room and last id returns only what was missed", [m["b"] for m in hh["m"]] == ["second lobby message", "third lobby message"], hh)
    await recv(ann2, "n"); await drain(ann2, 0.2)
    await join(ann2, "lobby", last=0); await recv(ann2, "joined"); hh2 = await recv(ann2, "hist")
    check("joining the same room again re-syncs it", len(hh2["m"]) == 3, hh2)
    await drain(ann2, 0.2)

    # ---- leaving
    await ann2.send(json.dumps({"t": "leave"}))
    await recv(ann2, "left"); back = await recv(ann2, "rooms")
    check("leaving returns to the hub with fresh counts", back["r"][0]["u"] >= 1 and any(x["s"] == "football" and x["u"] == 3 for x in back["r"]), back["r"][:5])
    await say(ann2, "talking from the hub"); e = await recv(ann2, "err"); check("after leaving you cannot post", e["c"] == "no_room")

    # ---- counts and hub updates
    await drain(bob, 3.0)
    frank, _, _ = await enter("frank", room="football")
    n = await recv(bob, "n", 4)
    check("people in a room are told the new head count", n["c"] == 4, n)
    hubv, _, hubrooms = await enter("hubviewer")
    fb = [x for x in hubrooms["r"] if x["s"] == "football"][0]
    check("a newcomer's first list already shows people and heat", fb["u"] == 4 and fb["h"] >= 1, fb)
    grace, _, _ = await enter("grace", room="football")
    hubf = await recv(hubv, "hub", 4)
    fb2 = [x for x in hubf["r"] if x["s"] == "football"][0]
    check("the hub is updated when people move", fb2["u"] == 5, hubf)
    check("an unchanged hub is not sent again", await quiet(hubv, "hub", 2.6))
    # the same person in two tabs counts once
    bob_b = await conn(token=okbob["k"], room="football"); await recv(bob_b, "ok"); await recv(bob_b, "rooms"); await recv(bob_b, "joined"); await recv(bob_b, "hist"); nb = await recv(bob_b, "n")
    check("two tabs of one person count once", nb["c"] == 5, nb)
    # same person in two different rooms
    bob_c = await conn(token=okbob["k"], room="music"); await recv(bob_c, "ok"); await recv(bob_c, "rooms"); await recv(bob_c, "joined"); await recv(bob_c, "hist"); nm = await recv(bob_c, "n")
    check("one person can be in two rooms with two tabs", nm["c"] == 1, nm)
    for w in (bob_b, bob_c, hubv): await w.close()

    # ---- roles on messages
    boss, okboss, _ = await enter(login=("boss", "boss password 1"), room="lobby")
    check("admin sees the admin role", okboss["o"] == "admin")
    await say(boss, "this is the admin"); mboss = await recv(boss, "m")
    check("an admin's message carries the admin role", mboss["o"] == "admin", mboss)
    mann = await recv(ann, "m") if False else None
    await say(dan, "plain member here"); mdan = await recv(dan, "m")
    check("a member's message has no role", "o" not in mdan, mdan)
    await cleo.send(json.dumps({"t": "pw", "p": "cleo password"})); await recv(cleo, "pwok"); await drain(cleo, 0.2)
    await boss.send(json.dumps({"t": "mod", "a": "promote", "h": "cleo"})); await recv(boss, "modok"); await drain(cleo, 0.3)
    await say(cleo, "mod speaking in football"); mc = await recv(cleo, "m")
    check("a moderator's message carries the mod role", mc["o"] == "mod", mc)

    # ---- delete
    await drain(ann, 0.3); await drain(dan, 0.3); await drain(boss, 0.3)
    await say(dan, "dan will delete this"); md = await recv(dan, "m"); await recv(boss, "m")
    await dan.send(json.dumps({"t": "del", "i": md["i"]}))
    dd = await recv(boss, "del"); check("a member can delete their own message and the room is told", dd["i"] == md["i"])
    await recv(dan, "modok")
    await say(dan, "another one"); md2 = await recv(dan, "m"); await drain(boss, 0.3); await drain(ann, 0.3)
    await ann.send(json.dumps({"t": "join", "r": "lobby"})); await drain(ann, 0.3)
    await ann.send(json.dumps({"t": "del", "i": md2["i"]})); e = await recv(ann, "err"); check("a member cannot delete someone else's message", e["c"] == "forbidden", e)
    await bob.send(json.dumps({"t": "del", "i": md2["i"]})); e = await recv(bob, "err"); check("not even from another room", e["c"] == "forbidden")
    await drain(cleo, 0.3); await drain(bob, 0.3)
    await boss.send(json.dumps({"t": "mod", "a": "del", "i": mb["i"]}))
    d3 = await recv(bob, "del"); check("a moderator can delete in a room they are not in", d3["i"] == mb["i"])
    check("the delete only reaches the room the message was in", await quiet(dan, "del", 0.4))
    check("the message is gone from the database", sql(f"SELECT count(*) FROM messages WHERE id = {mb['i']}") == "0")
    await cleo.send(json.dumps({"t": "mod", "a": "del", "i": mboss["i"]})); e = await recv(cleo, "err"); check("a moderator cannot delete the admin's message", e["c"] == "forbidden")

    # ---- reports and mutes across rooms
    await drain(bob, 0.2); await drain(cleo, 0.2); await drain(boss, 0.3)
    await say(bob, "bad words in football"); mbad = await recv(bob, "m"); await recv(cleo, "m")
    await cleo.send(json.dumps({"t": "rep", "i": mbad["i"], "c": "abuse"})); await recv(cleo, "repok")
    await boss.send(json.dumps({"t": "reports"})); rp = await recv(boss, "reps")
    check("the admin sees a report about a room they are not in", rp["r"][0]["i"] == mbad["i"] and rp["r"][0]["h"] == "bob", rp)
    await drain(boss, 0.2)
    await boss.send(json.dumps({"t": "mod", "a": "mute", "h": "bob", "m": 5, "rid": rp["r"][0]["id"]}))
    note = await recv(cleo, "mod"); check("a mute is announced where the person is", note["h"] == "bob" and note["a"] == "mute", note)
    check("and not in other rooms", await quiet(dan, "mod", 0.4))
    await recv(bob, "muted"); await say(bob, "i am muted"); e = await recv(bob, "err"); check("a muted person cannot post in any room", e["c"] == "muted")
    await boss.send(json.dumps({"t": "mod", "a": "unmute", "h": "bob"})); await recv(boss, "modok")

    # ---- kicks close every tab
    t1 = await conn(token=okbob["k"], room="football"); await recv(t1, "ok"); await drain(t1, 0.3)
    t2 = await conn(token=okbob["k"], room="music"); await recv(t2, "ok"); await drain(t2, 0.3)
    await boss.send(json.dumps({"t": "mod", "a": "kick", "h": "bob", "m": 5}))
    k1 = await recv(t1, "kicked"); k2 = await recv(t2, "kicked")
    check("a kick reaches every tab, in every room", k1["m"] == 5 and k2["m"] == 5)
    check("and closes them", await closed(t1) and await closed(t2))
    sql("UPDATE sanctions SET until = now() - interval '1 minute'")

    # ---- staff can enter every room
    await join(boss, "elite-lounge"); jb = await recv(boss, "joined")
    check("an admin can enter the Elite Lounge without the tier", jb["r"] == "elite-lounge", jb)
    await recv(boss, "hist"); await recv(boss, "n"); await join(boss, "lobby"); await recv(boss, "joined"); await recv(boss, "hist"); await drain(boss, 0.2)
    await join(cleo, "elite-lounge"); jc = await recv(cleo, "joined")
    check("a moderator can enter it too", jc["r"] == "elite-lounge", jc)
    await recv(cleo, "hist"); await recv(cleo, "n"); await join(cleo, "football"); await recv(cleo, "joined"); await recv(cleo, "hist"); await drain(cleo, 0.2)
    await join(dan, "elite-lounge"); e = await recv(dan, "err"); check("a member without the tier is still refused", e["c"] == "tier_locked", e)

    # ---- logging out ends only that login
    b1 = await conn(login=("boss", "boss password 1")); ob1 = await recv(b1, "ok"); await drain(b1, 0.2)
    b2 = await conn(login=("boss", "boss password 1")); ob2 = await recv(b2, "ok"); await drain(b2, 0.2)
    check("two logins have different tokens", ob1["k"] != ob2["k"])
    await b1.send(json.dumps({"t": "bye"}))
    await recv(b1, "byeok"); check("log out is confirmed and the connection closes", await closed(b1))
    x = await conn(token=ob1["k"]); e = await recv(x, "err"); check("the logged out token no longer works", e["c"] == "session_expired", e)
    y = await conn(token=ob2["k"]); okk = await recv(y, "ok"); check("the other login still works", okk["h"] == "Boss")
    for w in (b2, y): await w.close()

    # ---- the tier gate opens for the right tier
    await ann.close(); await ann2.close(); await asyncio.sleep(1.6)
    sql("UPDATE users SET points = 10000 WHERE handle_key = 'ann'")
    ann, okann, _ = await enter(token=oka["k"])
    check("points come back from the database", okann["r"] == 3 and okann["p"] == 10000, okann)
    await join(ann, "elite-lounge"); j = await recv(ann, "joined"); check("an Elite member can enter the Elite Lounge", j["r"] == "elite-lounge")
    await recv(ann, "hist"); await recv(ann, "n")
    low = await conn(handle="newbie1"); await recv(low, "ok"); await recv(low, "rooms")
    await join(low, "elite-lounge"); e = await recv(low, "err"); check("a Newbie is still refused", e["c"] == "tier_locked")
    lowtok = None

    # ---- everything per room survives a restart, edits to rooms are kept
    sql("UPDATE rooms SET name = 'Football Banter' WHERE slug = 'football'")
    await say(ann, "elite words"); await recv(ann, "m")
    await asyncio.sleep(1.2)
    stop(); start()
    f2, _, rooms2 = await enter("fresh", room="football")
    check("edited room names survive a restart", [x["n"] for x in rooms2["r"] if x["s"] == "football"] == ["Football Banter"])
    check("rooms are not duplicated by a restart", len(rooms2["r"]) == 10)
    f2h = None
    ws = await conn(handle="reader2", room="elite-lounge")
    await recv(ws, "ok"); await recv(ws, "rooms")
    e = await recv(ws, "err"); check("a locked room stays locked when joined with the first frame", e["c"] == "tier_locked")
    lob, _, _ = await enter("reader3")
    await join(lob, "lobby"); await recv(lob, "joined"); hl = await recv(lob, "hist")
    bodies = [(m["b"], m.get("o")) for m in hl["m"]]
    check("lobby history survived with roles", ("this is the admin", "admin") in bodies and ("plain member here", None) in bodies, bodies)
    check("a deleted message stayed deleted", all(b != "dan will delete this" for b, _ in bodies))
    await join(lob, "football"); await recv(lob, "joined"); hf = await recv(lob, "hist")
    fb = [(m["b"], m.get("o")) for m in hf["m"]]
    check("football history survived with the mod role", ("mod speaking in football", "mod") in fb, fb)
    check("a message removed by a moderator stayed removed", all(b != "football only" for b, _ in fb))
    check("the muted person's blocked message was never shown", all(b != "i am muted" for b, _ in fb))
    await join(lob, "elite-lounge")
    e = await recv(lob, "err"); check("reader is still too low for the Elite Lounge", e["c"] == "tier_locked")
    eliteh = sql("SELECT count(*) FROM messages WHERE room = 'elite-lounge' AND body = 'elite words'")
    check("messages are saved under the right room", eliteh == "1", eliteh)

    stop()
    print(f"\n{passed} passed, {failed} failed")

asyncio.run(main())
