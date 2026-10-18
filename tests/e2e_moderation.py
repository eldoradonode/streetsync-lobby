import asyncio, json, os, signal, subprocess, time, urllib.request, websockets

PORT = 8096
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
    proc = subprocess.Popen([BIN], env=env, stdout=open("/tmp/lobby_mod.log", "a"), stderr=subprocess.STDOUT)
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

async def main():
    sql("TRUNCATE users, sessions, messages, sanctions, reports, mod_log RESTART IDENTITY CASCADE")
    # a normal guest called Boss exists before the admin setup runs
    start()
    g = await join("Boss"); old = await recv(g, "ok"); await g.close()
    stop()

    start({"ADMIN_HANDLE": "Boss", "ADMIN_PASSWORD": "boss password 1"})
    x = await join(token=old["k"]); e = await recv(x, "err")
    check("adopting an existing guest handle ends its old sessions", e["c"] == "session_expired", e)
    boss = await join(login=("boss", "boss password 1")); okb = await ready(boss)
    check("admin logs in with the configured password and has the admin role", okb["h"] == "Boss" and okb["o"] == "admin" and okb["g"] is True, okb)
    x = await join(login=("boss", "wrong password")); e = await recv(x, "err"); check("wrong admin password refused", e["c"] == "bad_login")
    x = await join("BOSS"); e = await recv(x, "err"); check("nobody can claim the admin handle", e["c"] == "handle_taken")

    ann = await join("ann"); oka = await ready(ann)
    bob = await join("bob"); okbob = await ready(bob)
    cleo = await join("cleo"); okc = await ready(cleo)
    dan = await join("dan"); await ready(dan)
    check("normal users have no role in their me frame", "o" not in oka and "o" not in okbob)
    for w in (boss, ann, bob, cleo, dan): await drain(w, 0.2)

    # guests have no powers
    await mod(ann, a="mute", h="bob", m=5); e = await recv(ann, "err"); check("a normal user cannot moderate", e["c"] == "forbidden")
    await ann.send(json.dumps({"t": "reports"})); e = await recv(ann, "err"); check("a normal user cannot list reports", e["c"] == "forbidden")
    await mod(ann, a="promote", h="bob"); e = await recv(ann, "err"); check("a normal user cannot promote", e["c"] == "forbidden")

    # mute
    await mod(boss, a="mute", h="Bob", m=10)
    mu = await recv(bob, "muted"); check("the muted user is told", mu["m"] == 10, mu)
    ann_note = await recv(ann, "mod"); check("the room sees who was muted", ann_note == {"t": "mod", "a": "mute", "h": "bob", "m": 10} or (ann_note["a"] == "mute" and ann_note["h"].lower() == "bob" and ann_note["m"] == 10), ann_note)
    ok = await recv(boss, "modok"); check("the moderator gets a confirmation", ok["a"] == "mute")
    await drain(ann, 0.2)
    await say(bob, "can anyone hear me")
    e = await recv(bob, "err"); check("a muted user cannot post", e["c"] == "muted" and e["m"] == 10, e)
    check("nobody sees the muted message", await quiet(ann, "m"))
    await mod(boss, a="mute", h="boss", m=5); e = await recv(boss, "err"); check("cannot act on yourself", e["c"] == "forbidden")
    await mod(boss, a="mute", h="nobody", m=5); e = await recv(boss, "err"); check("unknown user", e["c"] == "no_such_user")
    stop(); start({"ADMIN_HANDLE": "Boss", "ADMIN_PASSWORD": "boss password 1"})
    bob = await join(token=okbob["k"]); await ready(bob)
    await say(bob, "still muted after restart"); e = await recv(bob, "err"); check("a mute survives a restart", e["c"] == "muted", e)
    boss = await join(login=("boss", "boss password 1")); await ready(boss)
    await mod(boss, a="unmute", h="bob"); um = await recv(bob, "unmuted"); check("unmute tells the user", um["t"] == "unmuted")
    await recv(boss, "modok")
    await say(bob, "i can talk again"); m = await recv(bob, "m"); check("after unmute the user can post", m["b"] == "i can talk again")
    await mod(boss, a="unmute", h="bob"); e = await recv(boss, "err"); check("unmuting someone who is not muted", e["c"] == "not_muted")

    # kick
    ann = await join(token=oka["k"]); await ready(ann)
    await mod(boss, a="kick", h="ann", m=5)
    kk = await recv(ann, "kicked"); check("the kicked user is told how long", kk["m"] == 5, kk)
    check("the kicked connection is closed", await closed(ann))
    x = await join(token=oka["k"]); e = await recv(x, "err")
    check("a kicked user cannot come back yet", e["c"] == "kicked" and 1 <= e["m"] <= 5, e)
    stop(); start({"ADMIN_HANDLE": "Boss", "ADMIN_PASSWORD": "boss password 1"})
    x = await join(token=oka["k"]); e = await recv(x, "err"); check("a kick survives a restart", e["c"] == "kicked", e)
    sql("UPDATE sanctions SET until = now() - interval '1 minute' WHERE kind = 'kick'")
    ann = await join(token=oka["k"]); okann = await ready(ann); check("after the kick ends the user can return", okann["h"] == "ann")
    boss = await join(login=("boss", "boss password 1")); await ready(boss)
    bob = await join(token=okbob["k"]); await ready(bob)
    cleo = await join(token=okc["k"]); await ready(cleo)
    dan = await join("dan2"); await ready(dan)
    for w in (boss, ann, bob, cleo, dan): await drain(w, 0.2)

    # roles
    await mod(boss, a="promote", h="bob"); e = await recv(boss, "err"); check("promoting needs a password on the account", e["c"] == "needs_password", e)
    await cleo.send(json.dumps({"t": "pw", "p": "cleo password"})); await recv(cleo, "pwok"); await drain(cleo, 0.2)
    await mod(boss, a="promote", h="cleo")
    me = await recv_me(cleo, lambda m: m.get("o") == "mod"); check("a promoted user is told their new role", me["o"] == "mod", me)
    await recv(boss, "modok")
    await mod(cleo, a="promote", h="ann"); e = await recv(cleo, "err"); check("a moderator cannot promote", e["c"] == "forbidden")
    await mod(cleo, a="mute", h="boss", m=5); e = await recv(cleo, "err"); check("a moderator cannot act on the admin", e["c"] == "forbidden")
    await mod(cleo, a="mute", h="ann", m=1); await recv(cleo, "modok"); await recv(ann, "muted")
    check("a moderator can mute a normal user", True)
    await mod(cleo, a="unmute", h="ann"); await recv(cleo, "modok")
    await mod(boss, a="demote", h="cleo"); me = await recv_me(cleo, lambda m: "o" not in m); check("demoting removes the role", "o" not in me, me)
    await recv(boss, "modok")
    await mod(cleo, a="mute", h="ann", m=1); e = await recv(cleo, "err"); check("a demoted moderator has no powers", e["c"] == "forbidden")
    await mod(boss, a="demote", h="ann"); e = await recv(boss, "err"); check("cannot demote someone who is not a moderator", e["c"] == "forbidden")
    await mod(boss, a="promote", h="cleo"); await recv_me(cleo, lambda m: m.get("o") == "mod"); await recv(boss, "modok")
    for w in (boss, ann, bob, cleo, dan): await drain(w, 0.2)

    # reports
    await say(bob, "you are all idiots"); mb = await recv(dan, "m"); await drain(bob, 0.2)
    await dan.send(json.dumps({"t": "rep", "i": mb["i"], "c": "abuse"}))
    await recv(dan, "repok")
    rn = await recv(boss, "rn"); check("moderators see the open report count", rn["c"] == 1, rn)
    rn2 = await recv(cleo, "rn"); check("every moderator is told", rn2["c"] == 1)
    await dan.send(json.dumps({"t": "rep", "i": mb["i"], "c": "spam"})); e = await recv(dan, "err"); check("the same report twice is refused", e["c"] == "already_reported")
    await bob.send(json.dumps({"t": "rep", "i": mb["i"], "c": "abuse"})); e = await recv(bob, "err"); check("cannot report yourself", e["c"] == "self_report")
    await dan.send(json.dumps({"t": "rep", "i": mb["i"], "c": "rude"})); e = await recv(dan, "err"); check("unknown category refused", e["c"] == "bad_report")
    await dan.send(json.dumps({"t": "rep", "i": 55555555, "c": "spam"})); e = await recv(dan, "err"); check("report of a missing message refused", e["c"] == "report_gone")
    await boss.send(json.dumps({"t": "reports"})); rp = await recv(boss, "reps")
    r0 = rp["r"][0]
    check("the report list has the copied message, reporter and reason", r0["b"] == "you are all idiots" and r0["by"] == "dan2" and r0["h"] == "bob" and r0["c"] == "abuse" and r0["i"] == mb["i"], rp)
    await mod(boss, a="dismiss", id=r0["id"]); await recv(boss, "modok"); rn = await recv(boss, "rn")
    check("dismissing closes the report", rn["c"] == 0, rn)
    await mod(boss, a="dismiss", id=r0["id"]); e = await recv(boss, "err"); check("a closed report cannot be dismissed again", e["c"] == "report_gone")
    await dan.send(json.dumps({"t": "rep", "i": mb["i"], "c": "abuse"})); e = await recv(dan, "err"); check("a dismissed report cannot be filed again by the same user", e["c"] == "already_reported")
    # acting on a report closes it
    await ann.send(json.dumps({"t": "rep", "i": mb["i"], "c": "abuse"})); await recv(ann, "repok"); await drain(boss, 0.3)
    await boss.send(json.dumps({"t": "reports"})); rp = await recv(boss, "reps")
    await mod(boss, a="mute", h="bob", m=2, rid=rp["r"][0]["id"])
    rn = await recv(boss, "rn"); check("acting on a report closes it", rn["c"] == 0, rn)
    await mod(boss, a="unmute", h="bob"); await drain(boss, 0.3); await drain(bob, 0.3)
    # report rate limit: dan2 already filed one report, so nine more are allowed
    for w in (boss, ann, bob, cleo, dan): await drain(w, 0.3)
    for w, name, n in ((boss, "boss", 4), (bob, "bob", 4), (cleo, "cleo", 3)):
        for i in range(n):
            await say(w, f"filler {name} {i}")
    seen = []
    end = time.time() + 1.2
    while time.time() < end:
        try:
            m = json.loads(await asyncio.wait_for(dan.recv(), 0.3))
            if m["t"] == "m" and m["b"].startswith("filler"): seen.append(m["i"])
        except asyncio.TimeoutError: break
    res = []
    for i in seen:
        await dan.send(json.dumps({"t": "rep", "i": i, "c": "spam"}))
        while True:
            m = await recv(dan)
            if m["t"] in ("repok", "err"):
                res.append(m["t"] if m["t"] == "repok" else m["c"]); break
    check("report rate limit stops reports after ten in an hour", len(seen) == 11 and res == ["repok"] * 9 + ["slow_down"] * 2, (len(seen), res))
    for w in (boss, ann, bob, cleo, dan): await drain(w, 0.3)

    # delete
    for w in (boss, ann, bob, cleo, dan): await drain(w, 0.2)
    await asyncio.sleep(1.3)
    await say(bob, "delete me please"); md = await recv(boss, "m"); await drain(cleo, 0.2)
    await mod(boss, a="del", i=md["i"])
    d1 = await recv(dan, "del"); check("everyone is told a message was removed", d1["i"] == md["i"])
    await recv(boss, "modok")
    await say(boss, "admin words"); ma = await recv(cleo, "m")
    await mod(cleo, a="del", i=ma["i"]); e = await recv(cleo, "err"); check("a moderator cannot remove the admin's message", e["c"] == "forbidden")
    await mod(ann, a="del", i=ma["i"]); e = await recv(ann, "err"); check("a normal user cannot remove messages", e["c"] == "forbidden")
    await mod(boss, a="del", i=999999999); e = await recv(boss, "err"); check("removing an unknown message", e["c"] == "msg_gone")
    await asyncio.sleep(1.3)
    await say(bob, "quick delete before saving"); mq = await recv(boss, "m")
    await mod(boss, a="del", i=mq["i"]); await recv(boss, "modok")
    await asyncio.sleep(1.0)
    check("a message removed before it was saved is never saved", sql(f"SELECT count(*) FROM messages WHERE id = {mq['i']}") == "0")
    check("a removed message is gone from the database", sql(f"SELECT count(*) FROM messages WHERE id = {md['i']}") == "0")
    fresh = await join("eve"); await recv(fresh, "ok"); hh = await recv(fresh, "hist")
    check("new joiners do not see removed messages", all(m["i"] not in (md["i"], mq["i"]) for m in hh["m"]), [m["b"] for m in hh["m"]])

    # log
    acts = set(sql("SELECT DISTINCT action FROM mod_log").split("\n"))
    check("every kind of action is logged", {"mute", "unmute", "kick", "del", "promote", "demote", "dismiss"} <= acts, acts)
    check("the log names who did it", sql("SELECT count(*) FROM mod_log WHERE actor = 'Boss'") != "0" and sql("SELECT count(*) FROM mod_log WHERE actor = 'cleo'") != "0")
    stop()
    print(f"\n{passed} passed, {failed} failed")

asyncio.run(main())
