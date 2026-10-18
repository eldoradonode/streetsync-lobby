import asyncio, json, os, signal, subprocess, time, urllib.request, websockets

PORT = 8111
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
    proc = subprocess.Popen([BIN], env=env, stdout=open("/tmp/lobby_admin.log", "a"), stderr=subprocess.STDOUT)
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

async def main():
    sql("TRUNCATE users, sessions, messages, sanctions, reports, mod_log RESTART IDENTITY CASCADE")
    sql("DELETE FROM rooms WHERE kind <> 'official'")
    start(ENV)
    sql("UPDATE users SET credits = 500 WHERE handle_key = 'boss'")
    boss = await join(login=("boss", "boss password 1")); await ready(boss)
    ann = await join("ann"); oka = await ready(ann); await go_pw(ann, "ann password 1")
    bob = await join("bob"); await ready(bob); await go_pw(bob, "bob password 1")
    cleo = await join("cleo"); await ready(cleo); await go_pw(cleo, "cleo password 1")
    dan = await join("dan"); await ready(dan)           # no password
    for w in (boss, ann, bob, cleo, dan): await drain(w, 0.2)
    await send(boss, t="mod", a="promote", h="cleo"); await recv(boss, "modok")
    await recv_me(cleo, lambda m: m.get("o") == "mod"); await drain(cleo, 0.2)

    # ---- ordinary users are locked out of everything
    for t in ("astats", "ausers", "astaff", "alog", "arooms"):
        check(f"an ordinary user cannot use {t}", await forbidden(ann, t))
    for a, kw in (("promote", dict(h="bob")), ("demote", dict(h="cleo")), ("clearbio", dict(h="bob")), ("grant", dict(h="bob", m=50)), ("mute", dict(h="bob", m=5))):
        await send(ann, t="mod", a=a, **kw); e = await recv(ann, "err"); check(f"an ordinary user cannot {a}", e["c"] == "forbidden", e)
    await send(ann, t="rclose", r="lobby"); e = await recv(ann, "err"); check("an ordinary user cannot close a room they do not own", e["c"] == "not_host", e)

    # ---- moderators: allowed the moderation views, never the admin ones
    st = await ask(cleo, "astats")
    check("a moderator gets moderation stats", st["t"] == "astats" and "rep" in st and "on" in st and "mu" in st, st)
    check("a moderator does not get platform or money numbers", not ({"us", "cr", "md", "ud", "mo", "ad", "m1"} & set(st)), st)
    check("a moderator cannot list staff", await forbidden(cleo, "astaff"))
    check("a moderator cannot read the audit log", await forbidden(cleo, "alog"))
    us = await ask(cleo, "ausers", h="ann")
    check("a moderator can find users", [u["h"] for u in us["u"]] == ["ann"], us)
    check("a moderator does not see credits or last seen", all("c" not in u and "ls" not in u for u in us["u"]), us)
    rm = await ask(cleo, "arooms"); check("a moderator can list rooms", rm["t"] == "arooms" and any(r["s"] == "lobby" for r in rm["r"]), rm)
    for a, kw in (("promote", dict(h="bob")), ("promote", dict(h="cleo")), ("demote", dict(h="cleo")), ("demote", dict(h="boss")), ("mute", dict(h="boss", m=5)), ("grant", dict(h="bob", m=50)), ("clearbio", dict(h="boss"))):
        await send(cleo, t="mod", a=a, **kw); e = await recv(cleo, "err")
        check(f"a moderator cannot {a} {kw['h']}", e["c"] == "forbidden", e)
    check("the moderator is still only a moderator", (await sql_role("cleo")) == "mod")

    # ---- admin sees everything
    st = await ask(boss, "astats")
    total = int(sql("SELECT count(*) FROM users"))
    check("admin stats: total users match the database", st["us"] == total, (st["us"], total))
    check("admin stats: moderators and admins counted", st["mo"] == 1 and st["ad"] == 1, st)
    check("admin stats: people online counted", st["on"] == 5, st["on"])
    check("admin stats: credits in circulation match the database", st["cr"] == int(sql("SELECT sum(credits) FROM users")), st["cr"])
    check("admin stats: seven days of messages and signups", len(st["md"]) == 7 and len(st["ud"]) == 7 and st["ud"][-1]["n"] == total, st["ud"])
    check("admin stats: accounts with passwords counted", st["pw"] == int(sql("SELECT count(*) FROM users WHERE pass_hash IS NOT NULL")), st["pw"])
    await say(ann, "hello admin stats"); await recv(ann, "m"); await asyncio.sleep(1.2)
    st = await ask(boss, "astats"); check("admin stats: messages in the last day", st["m1"] >= 1 and st["md"][-1]["n"] >= 1, st["md"])
    us = await ask(boss, "ausers", h="")
    check("an empty search lists the newest accounts first", [u["h"] for u in us["u"]][:2] == ["dan", "cleo"], [u["h"] for u in us["u"]])
    us = await ask(boss, "ausers", h="@BO")
    check("search ignores case and the @ sign", {u["h"] for u in us["u"]} == {"bob", "Boss"}, us)
    check("admin sees credits and last seen", all("c" in u and "ls" in u for u in us["u"]), us)
    us = await ask(boss, "ausers", h="%")
    check("wildcard characters are not special", len(us["u"]) <= 20 and us["q"] == "", us["q"])
    us = await ask(boss, "ausers", h="zzzz"); check("no match gives an empty list", us["u"] == [])
    sf = await ask(boss, "astaff")
    names = {s["h"]: s for s in sf["s"]}
    check("the staff list shows the admin and the moderator", set(names) == {"Boss", "cleo"} and names["Boss"]["o"] == "admin" and names["cleo"]["o"] == "mod", sf)
    check("it shows who promoted the moderator", names["cleo"].get("by") == "Boss" and names["cleo"].get("at"), names["cleo"])
    check("it shows who is online", names["cleo"]["on"] is True)

    # ---- promote and revoke
    await send(boss, t="mod", a="promote", h="dan"); e = await recv(boss, "err"); check("promoting an account without a password is refused", e["c"] == "needs_password", e)
    await send(boss, t="mod", a="promote", h="boss"); e = await recv(boss, "err"); check("admin cannot promote themselves", e["c"] == "forbidden", e)
    await send(boss, t="mod", a="demote", h="boss"); e = await recv(boss, "err"); check("admin cannot demote themselves", e["c"] == "forbidden", e)
    await send(boss, t="mod", a="promote", h="ann"); await recv(boss, "modok")
    me = await recv_me(ann, lambda m: m.get("o") == "mod"); check("the promoted user is told at once", me["o"] == "mod")
    check("the database has the new role", (await sql_role("ann")) == "mod")
    check("a new moderator can use the moderation views", (await ask(ann, "astats"))["t"] == "astats")
    check("a new moderator still cannot use admin views", await forbidden(ann, "astaff"))
    await send(ann, t="mod", a="promote", h="bob"); e = await recv(ann, "err"); check("and still cannot promote others", e["c"] == "forbidden", e)
    sf = await ask(boss, "astaff"); check("the staff list now includes the new moderator", {s["h"] for s in sf["s"]} == {"Boss", "cleo", "ann"})
    await send(boss, t="mod", a="demote", h="ann"); await recv(boss, "modok")
    me = await recv_me(ann, lambda m: "o" not in m); check("revoking tells the user at once", "o" not in me)
    check("a revoked moderator loses access immediately", await forbidden(ann, "astats") and await forbidden(ann, "arooms"))
    sf = await ask(boss, "astaff"); check("the staff list drops them", {s["h"] for s in sf["s"]} == {"Boss", "cleo"})
    await send(boss, t="mod", a="demote", h="bob"); e = await recv(boss, "err"); check("revoking someone who is not a moderator is refused", e["c"] == "forbidden", e)
    lg = await ask(boss, "alog")
    acts = [(l["x"], l.get("h")) for l in lg["l"]]
    check("promotions and revocations are in the audit log", ("promote", "ann") in acts and ("demote", "ann") in acts and ("promote", "cleo") in acts, acts)
    check("the audit log names who did it", all(l["a"] == "Boss" for l in lg["l"] if l["x"] in ("promote", "demote")), lg["l"][:3])

    # ---- sanctions show up in user search
    await send(cleo, t="mod", a="mute", h="bob", m=30); await recv(cleo, "modok")
    us = await ask(cleo, "ausers", h="bob"); check("a muted user shows their minutes left", us["u"][0].get("mu", 0) in (29, 30), us)

    # ---- bio
    await send(ann, t="bio", b="  Abuja\nbuilder   of\tthings  "); me = await recv_me(ann, lambda m: "bio" in m); await recv(ann, "biook"); check("bio is tidied and saved", me["bio"] == "Abuja builder of things", me)
    await ann.close(); await asyncio.sleep(0.3)
    ann = await join(token=oka["k"]); okr = await ready(ann)
    check("a returning login (saved token) works and brings the bio back", okr["h"] == "ann" and okr.get("bio") == "Abuja builder of things", okr)
    await drain(ann, 0.2)
    await send(bob, t="profile", h="ann"); pr = await recv(bob, "uprofile"); check("others see the bio on the profile", pr.get("bio") == "Abuja builder of things", pr)
    await send(ann, t="bio", b="x" * 161); e = await recv(ann, "err"); check("a bio over 160 characters is refused", e["c"] == "bad_bio", e)
    await send(bob, t="mod", a="clearbio", h="ann"); e = await recv(bob, "err"); check("an ordinary user cannot clear a bio", e["c"] == "forbidden", e)
    await send(cleo, t="mod", a="clearbio", h="ann"); await recv(cleo, "modok")
    me = await recv_me(ann, lambda m: "bio" not in m); check("a moderator can clear a bio and the owner is updated", "bio" not in me)
    check("clearing is logged", "clearbio" in {l["x"] for l in (await ask(boss, "alog"))["l"]})

    # ---- room categories and the directory
    await send(boss, t="croom", n="Night Owls", k="public", g="gist", c="Sport"); ok = await recv(boss, "croomok")
    check("a room can be created with a category", sql("SELECT category FROM rooms WHERE slug = '%s'" % ok["s"]) == "Sport")
    await send(boss, t="mod", a="grant", h="cleo", m=100); await recv(boss, "modok"); await drain(cleo, 0.3)
    await send(boss, t="mod", a="promote", h="ann"); await recv(boss, "modok"); await recv_me(ann, lambda m: m.get("o") == "mod")
    await send(boss, t="mod", a="grant", h="ann", m=100); await recv(boss, "modok"); await drain(ann, 0.3)
    await send(cleo, t="croom", n="Quiet Den", k="public", g=""); ok2 = await recv(cleo, "croomok")
    check("no category means General", sql("SELECT category FROM rooms WHERE slug = '%s'" % ok2["s"]) == "General")
    await send(cleo, t="croom", n="Odd Place", k="public", c="Nonsense"); e = await recv(cleo, "err"); check("an unknown category is refused", e["c"] == "bad_category", e)
    await send(ann, t="croom", n="Secret Spot", k="private", c="Tech"); ok3 = await recv(ann, "croomok")
    await drain(boss, 0.3)
    d = await ask(dan, "dir")
    slugs = {r["s"] for r in d["r"]}
    check("the directory lists public rooms even with nobody inside", ok["s"] in slugs and ok2["s"] in slugs, slugs)
    check("the directory never lists private rooms", ok3["s"] not in slugs)
    check("the directory carries the category", {r["s"]: r["c"] for r in d["r"]}[ok["s"]] == "Sport")
    hub_rooms = [r["s"] for r in (await ask(dan, "find", expect="found", n="night"))["r"]]
    check("search still finds the room too", ok["s"] in hub_rooms)

    # ---- staff can close rooms; admin room list shows private rooms
    ar = await ask(boss, "arooms"); by = {r["s"]: r for r in ar["r"]}
    check("the room list shows private rooms to staff", ok3["s"] in by and by[ok3["s"]]["k"] == "private" and by[ok3["s"]]["o"] == "ann", by.get(ok3["s"]))
    check("it shows official rooms without an owner", by["lobby"]["k"] == "official" and "o" not in by["lobby"])
    await send(cleo, t="rclose", r="lobby"); e = await recv(cleo, "err"); check("official rooms can not be closed", e["c"] == "not_host", e)
    await send(cleo, t="rclose", r=ok["s"]); await recv(cleo, "modok")
    check("a moderator can close a community room", sql("SELECT count(*) FROM rooms WHERE slug = '%s'" % ok["s"]) == "0")
    check("and it is logged", "close_room" in {l["x"] for l in (await ask(boss, "alog"))["l"]})

    stop()
    print(f"\n{passed} passed, {failed} failed")
    raise SystemExit(1 if failed else 0)

async def sql_role(handle): return sql("SELECT role FROM users WHERE handle_key = '%s'" % handle)

asyncio.run(main())
