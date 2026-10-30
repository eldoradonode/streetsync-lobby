import asyncio, json, os, signal, subprocess, time, urllib.request, websockets

PORT = 8097
URL = f"ws://localhost:{PORT}/ws"
DB = "postgres://lobby:lobby@localhost/lobbytest?sslmode=disable"
ENV = dict(os.environ, PORT=str(PORT), DATABASE_URL=DB, HASH_ITERATIONS="1000", REQUIRE_PASSWORD="0", FLUSH_MS="500")
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
    proc = subprocess.Popen([os.environ.get("LOBBY_BIN", os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "lobby-test"))], env=ENV, stdout=open("/tmp/lobby2.log", "a"), stderr=subprocess.STDOUT)
    for _ in range(50):
        try:
            urllib.request.urlopen(f"http://localhost:{PORT}/healthz", timeout=1); return
        except Exception: time.sleep(0.1)
    raise SystemExit("server did not start")

def stop():
    proc.send_signal(signal.SIGTERM); proc.wait(timeout=15)

async def recv(ws, t=None, timeout=3):
    while True:
        m = json.loads(await asyncio.wait_for(ws.recv(), timeout))
        if t is None or m["t"] == t: return m

async def quiet(ws, t, wait=0.7):
    """True if no frame of type t arrives within wait seconds."""
    end = time.time() + wait
    while time.time() < end:
        try:
            m = json.loads(await asyncio.wait_for(ws.recv(), max(0.05, end - time.time())))
            if m["t"] == t: return False
        except asyncio.TimeoutError: break
    return True

async def points_quiet(ws, points, wait=0.6):
    """True when no me frame shows a points total other than `points`.
    Me frames that only change the credit balance are fine: credits arrive a moment later, from the database."""
    end = time.time() + wait
    while time.time() < end:
        try:
            m = json.loads(await asyncio.wait_for(ws.recv(), max(0.05, end - time.time())))
            if m["t"] == "me" and m["p"] != points: return False
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

async def hi(handle=None, token=None, last=0, login=None):
    ws = await websockets.connect(URL)
    if login: await ws.send(json.dumps({"t": "login", "h": login[0], "p": login[1], "r": "lobby"}))
    elif token: await ws.send(json.dumps({"t": "hi", "k": token, "l": last, "r": "lobby"}))
    else: await ws.send(json.dumps({"t": "hi", "h": handle, "r": "lobby"}))
    return ws

async def main():
    sql("TRUNCATE users, sessions, messages RESTART IDENTITY CASCADE")
    start()

    # sign in as a guest
    a = await hi("@Ada")
    ok = await recv(a, "ok")
    check("guest claim returns ok with 64 char token", ok["h"] == "Ada" and len(ok["k"]) == 64)
    check("new user starts at Newbie, 0 points, no password", ok["r"] == 0 and ok["p"] == 0 and ok["x"] == 300 and ok["s"] == 0 and ok["g"] is False, ok)
    hist = await recv(a, "hist"); check("empty history", hist["m"] == [])
    n = await recv(a, "n"); check("online count 1", n["c"] == 1)
    tokA = ok["k"]

    for h in ["ada", "ADA"]:
        x = await hi(h); e = await recv(x, "err"); check("handle_taken " + h, e["c"] == "handle_taken")
    for bad in ["ab", "has space", "waytoolonghandle_here", "x!y"]:
        x = await hi(bad); e = await recv(x, "err"); check("bad_handle " + bad, e["c"] == "bad_handle")
    x = await websockets.connect(URL); await x.send(json.dumps({"t": "m", "b": "sneaky"}))
    e = await recv(x, "err"); check("first frame must be hi or login", e["c"] == "bad_hello")
    x = await websockets.connect(URL); await x.send("not json")
    e = await recv(x, "err"); check("garbage first frame rejected", e["c"] == "bad_hello")

    # points, streak, tiers
    await a.send(json.dumps({"t": "m", "b": "hi"}))
    m = await recv(a, "m"); check("short message is delivered", m["b"] == "hi" and m["r"] == 0)
    check("short message earns nothing", await points_quiet(a, 0))
    await a.send(json.dumps({"t": "m", "b": "hello lobby"}))
    await recv(a, "m"); me = await recv_me(a, lambda m: m["p"] > 0)
    check("first scoring message: 1 point + 10 streak bonus", me["p"] == 11 and me["s"] == 1, me)
    await a.send(json.dumps({"t": "m", "b": "Hello Lobby"}))
    m = await recv(a, "m"); check("repeat is delivered", m["b"] == "Hello Lobby")
    check("repeat earns nothing", await points_quiet(a, 11))

    # second tab with the same token
    a2 = await hi(token=tokA)
    ok2 = await recv(a2, "ok"); h2 = await recv(a2, "hist")
    check("second tab resumes the same user", ok2["h"] == "Ada" and ok2["k"] == tokA and ok2["p"] == 11)
    check("second tab gets history", [m["b"] for m in h2["m"]] == ["hi", "hello lobby", "Hello Lobby"])
    n2 = await recv(a2, "n"); check("two tabs still count as one online", n2["c"] == 1, n2)

    # another user sees history and tiers; both of Ada's tabs get live messages
    b = await hi("bob"); okb = await recv(b, "ok"); hb = await recv(b, "hist")
    check("new joiner sees history", len(hb["m"]) == 3)
    await b.send(json.dumps({"t": "m", "b": "yo ada"}))
    m1 = await recv(a, "m"); m2 = await recv(a2, "m")
    check("both tabs receive live messages", m1["b"] == m2["b"] == "yo ada" and m1["h"] == "bob")

    # password
    await a.send(json.dumps({"t": "pw", "p": "short"}))
    e = await recv(a, "err"); check("weak password rejected", e["c"] == "weak_password")
    await a.send(json.dumps({"t": "pw", "p": "correct horse"}))
    me = await recv_me(a, lambda m: m["g"]); await recv(a, "pwok")
    check("password saved, me frame says protected", me["g"] is True)
    me2 = await recv_me(a2, lambda m: m["g"]); check("other tab told too", me2["g"] is True)
    await a.send(json.dumps({"t": "pw", "p": "another one 12"}))
    e = await recv(a, "err"); check("cannot overwrite a password through pw", e["c"] == "has_password")
    check("password stored hashed", sql("SELECT pass_hash FROM users WHERE handle_key='ada'").startswith("pbkdf2-sha256$1000$"))

    # login
    l = await hi(login=("ada", "correct horse")); okl = await recv(l, "ok")
    check("login with the right password", okl["h"] == "Ada" and okl["g"] is True and okl["k"] != tokA and okl["p"] == 11, okl)
    for who, pw, name in [("ada", "wrong password", "wrong password"), ("nobody", "whatever12", "unknown handle"), ("bob", "whatever12", "guest handle")]:
        x = await hi(login=(who, pw)); e = await recv(x, "err"); check("bad_login: " + name, e["c"] == "bad_login")
    c = await hi("cleo"); await recv(c, "ok"); await c.send(json.dumps({"t": "pw", "p": "cleo password"})); await recv(c, "pwok")
    for i in range(8):
        x = await hi(login=("cleo", "nope nope " + str(i))); e = await recv(x, "err")
    x = await hi(login=("cleo", "cleo password")); e = await recv(x, "err")
    check("locked out after 8 failures, even with the right password", e["c"] == "too_many_attempts", e)

    # tier up
    await b.close(); await asyncio.sleep(1.6)
    sql("UPDATE users SET points=299 WHERE handle_key='bob'")
    b = await hi(token=okb["k"]); okb2 = await recv(b, "ok")
    check("points reloaded from the database", okb2["p"] == 299 and okb2["r"] == 0, okb2)
    obs = await hi(token=tokA); await recv(obs, "ok")
    await b.send(json.dumps({"t": "m", "b": "level me up"}))
    m = await recv(b, "m"); up = await recv(b, "up"); meb = await recv_me(b, lambda m: m["p"] > 0)
    check("message carries the new tier", m["r"] == 1, m)
    check("tier up announced", up["h"] == "bob" and up["r"] == 1)
    check("me frame shows Regular", meb["r"] == 1 and meb["p"] == 300 and meb["x"] == 2000, meb)
    upo = await recv(obs, "up"); check("everyone sees the tier up", upo["h"] == "bob")

    # rate limit
    await asyncio.sleep(1.2)
    for i in range(12): await obs.send(json.dumps({"t": "m", "b": f"spam {i} spam"}))
    errs = 0; end = time.time() + 2
    while time.time() < end:
        try:
            m = json.loads(await asyncio.wait_for(obs.recv(), 0.4))
            if m["t"] == "err" and m["c"] == "slow_down": errs += 1
        except asyncio.TimeoutError: break
    check("rate limit kicks in", errs >= 5, errs)

    # resume only missed messages
    last_seen = 0
    r1 = await hi(token=tokA); await recv(r1, "ok"); hh = await recv(r1, "hist"); last_seen = hh["m"][-1]["i"]; await r1.close()
    await b.send(json.dumps({"t": "m", "b": "while you were away"}))
    await asyncio.sleep(0.4)
    r2 = await hi(token=tokA, last=last_seen); await recv(r2, "ok"); h3 = await recv(r2, "hist")
    check("resume returns only missed messages", [m["b"] for m in h3["m"]] == ["while you were away"], h3)

    # replies
    await asyncio.sleep(2)
    await drain(obs); await drain(b)
    await b.send(json.dumps({"t": "m", "b": "original message from bob about lunch"}))
    orig = await recv(obs, "m"); await recv(b, "m")
    await obs.send(json.dumps({"t": "m", "b": "sounds good bob", "q": orig["i"]}))
    rep = await recv(obs, "m"); repb = await recv(b, "m")
    check("reply carries the id, sender and snippet of the original",
          rep["q"] == orig["i"] and rep["qh"] == "bob" and rep["qb"] == "original message from bob about lunch" and rep["b"] == "sounds good bob", rep)
    check("everyone gets the same reply", repb == rep)
    await obs.send(json.dumps({"t": "m", "b": "ghost reply", "q": 987654321}))
    e = await recv(obs, "err"); check("reply to a message that does not exist is refused", e["c"] == "reply_gone", e)
    check("refused reply is not broadcast", await quiet(b, "m"))
    await b.send(json.dumps({"t": "m", "b": "z" * 150}))
    longm = await recv(obs, "m"); await recv(b, "m")
    await obs.send(json.dumps({"t": "m", "b": "that is long", "q": longm["i"]}))
    rl = await recv(obs, "m"); await recv(b, "m")
    check("long originals are cut to 80 characters plus an ellipsis", rl["qb"] == "z" * 80 + "\u2026", rl["qb"])
    await asyncio.sleep(1.2)
    await b.send(json.dumps({"t": "m", "b": "replying to a reply", "q": rep["i"]}))
    rr = await recv(obs, "m"); await recv(b, "m")
    check("reply to a reply quotes only that reply", rr["qb"] == "sounds good bob" and rr["qh"] == "Ada" and "q" in rr, rr)
    await obs.send(json.dumps({"t": "m", "b": "plain message after replies"}))
    pm = await recv(obs, "m"); await recv(b, "m")
    check("plain messages carry no reply fields", "q" not in pm and "qh" not in pm and "qb" not in pm, pm)
    reply_id = rep["i"]

    # last words survive a shutdown, and everything survives a restart
    w = await hi(token=tokA); await recv(w, "ok"); await recv(w, "hist")
    await asyncio.sleep(1.2)
    await w.send(json.dumps({"t": "m", "b": "last words before stop"}))
    await recv(w, "m")
    max_before = int(sql("SELECT COALESCE(max(id),0) FROM messages") or 0)
    stop()
    log = open("/tmp/lobby2.log").read()
    check("clean shutdown logged", "stopped cleanly" in log)
    check("message saved during shutdown", sql("SELECT count(*) FROM messages WHERE body='last words before stop'") == "1")
    check("points saved during shutdown", sql("SELECT points FROM users WHERE handle_key='bob'") == "304" and sql("SELECT points FROM users WHERE handle_key='ada'") == "20", (sql("SELECT handle_key, points FROM users ORDER BY id")))
    top = int(sql("SELECT max(id) FROM messages"))
    start()
    r = await hi(token=tokA); okr = await recv(r, "ok"); hr = await recv(r, "hist")
    check("session survives a restart", okr["h"] == "Ada" and okr["g"] is True and okr["p"] == 20, okr)
    check("history survives a restart", len(hr["m"]) == min(30, int(sql("SELECT count(*) FROM messages"))) and hr["m"][-1]["b"] == "last words before stop", (len(hr["m"]), hr["m"][-1:]))
    check("tiers survive in history", any(m["r"] == 1 for m in hr["m"]))
    kept = [m for m in hr["m"] if m["i"] == reply_id]
    check("replies survive a restart with their quote", len(kept) == 1 and kept[0]["q"] == orig["i"] and kept[0]["qh"] == "bob" and kept[0]["qb"].startswith("original message"), kept)
    await r.send(json.dumps({"t": "m", "b": "replying to an old one", "q": orig["i"]}))
    rq = await recv(r, "m"); check("can reply to a message loaded from the database", rq["q"] == orig["i"] and rq["qh"] == "bob", rq)
    await r.send(json.dumps({"t": "m", "b": "back after restart"}))
    m = await recv(r, "m"); check("ids skip ahead after a restart", m["i"] >= top + 1000, (m["i"], top))
    x = await hi(login=("ada", "correct horse")); okx = await recv(x, "ok"); check("login works after restart", okx["h"] == "Ada")

    # session expiry
    sql("UPDATE sessions SET last_used = now() - interval '40 days' WHERE token_hash = encode(sha256('" + tokA.replace("'", "") + "'::bytea), 'hex')")
    y = await hi(token=tokA); e = await recv(y, "err"); check("expired session rejected", e["c"] == "session_expired")
    y = await hi(token="deadbeef"); e = await recv(y, "err"); check("unknown token rejected", e["c"] == "session_expired")

    stop()
    print(f"\n{passed} passed, {failed} failed")

asyncio.run(main())
