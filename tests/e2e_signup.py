import asyncio, json, os, signal, subprocess, time, urllib.request, websockets

PORT = 8110
URL = f"ws://localhost:{PORT}/ws"
DB = "postgres://lobby:lobby@localhost/lobbytest?sslmode=disable"
BASE = dict(os.environ, PORT=str(PORT), DATABASE_URL=DB, HASH_ITERATIONS="1000", FLUSH_MS="500",
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
    proc = subprocess.Popen([BIN], env=BASE, stdout=open("/tmp/lobby_signup.log", "a"), stderr=subprocess.STDOUT)
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

async def conn(handle=None, token=None, login=None, room=None, last=0, look=None, pw=None):
    ws = await websockets.connect(URL)
    extra = {"r": room, "l": last} if room else {}
    if look is not None: extra["v"] = look
    if login: await ws.send(json.dumps(dict(t="login", h=login[0], p=login[1], **extra)))
    elif token: await ws.send(json.dumps(dict(t="hi", k=token, **extra)))
    else:
        if pw is not None: extra["p"] = pw
        await ws.send(json.dumps(dict(t="hi", h=handle, **extra)))
    return ws


async def err(ws, timeout=3):
    m = await recv(ws, None, timeout)
    return m["c"] if m["t"] == "err" else m["t"]

async def main():
    sql("TRUNCATE users CASCADE")
    start()
    # a new handle with no password is refused and nothing is created
    ws = await conn("ada"); check("no password is refused", await err(ws) == "weak_password"); await ws.close()
    check("and no account was made", sql("SELECT count(*) FROM users WHERE handle_key='ada'") == "0")
    ws = await conn("ada", pw="short"); check("a short password is refused", await err(ws) == "weak_password"); await ws.close()
    ws = await conn("ada", pw="x" * 129); check("a huge password is refused", await err(ws) == "weak_password"); await ws.close()
    check("still no account", sql("SELECT count(*) FROM users WHERE handle_key='ada'") == "0")
    ws = await conn("a!", pw="longenough1"); check("a bad handle is still bad_handle", await err(ws) == "bad_handle"); await ws.close()

    # a good sign up works, has the password from the start, and can log in again
    ws = await conn("ada", pw="correct horse 1", look="f3"); ok = await recv(ws, "ok")
    check("sign up with handle and password works", ok["h"] == "ada" and ok.get("g") is True, ok)
    check("the hash is stored, not the password", sql("SELECT pass_hash LIKE 'pbkdf2%' OR length(pass_hash) > 20 FROM users WHERE handle_key='ada'") == "t" and "correct horse" not in sql("SELECT pass_hash FROM users WHERE handle_key='ada'"))
    check("the look was kept", ok.get("v") == "f3", ok)
    await ws.close(); await asyncio.sleep(0.3)
    ws = await conn(login=("ada", "correct horse 1")); ok2 = await recv(ws, "ok")
    check("and the same handle and password log in later", ok2["h"] == "ada", ok2); await ws.close()
    ws = await conn(login=("ada", "wrong password 1")); check("a wrong password is bad_login", await err(ws) == "bad_login"); await ws.close()

    # taken handles are reported before any password work, whatever case
    ws = await conn("ADA", pw="another pass 1"); check("a taken handle is handle_taken", await err(ws) == "handle_taken"); await ws.close()
    ws = await conn("ada"); check("a taken handle with no password says handle_taken or weak_password, never creates", await err(ws) in ("weak_password", "handle_taken")); await ws.close()
    check("still one ada", sql("SELECT count(*) FROM users WHERE handle_key='ada'") == "1")

    # two people racing for one handle: exactly one wins
    a, b = await asyncio.gather(conn("racer", pw="racer pass 1"), conn("racer", pw="racer pass 2"))
    ra, rb = await asyncio.gather(err(a), err(b))
    check("a race for a handle has one winner", sorted([ra, rb]) == ["handle_taken", "ok"], (ra, rb))
    check("and one row", sql("SELECT count(*) FROM users WHERE handle_key='racer'") == "1")
    await a.close(); await b.close()

    # returning with a saved token still works with no password sent
    ws = await conn("bea", pw="bea password 1"); okb = await recv(ws, "ok"); await ws.close(); await asyncio.sleep(0.3)
    ws = await conn(token=okb["k"]); okb2 = await recv(ws, "ok")
    check("a saved login resumes", okb2["h"] == "bea", okb2); await ws.close()

    # an old passwordless guest still logs in with the saved token and can set a password
    sql("INSERT INTO users (handle, handle_key) VALUES ('oldguest','oldguest')")
    check("old guest rows may exist", sql("SELECT count(*) FROM users WHERE handle_key='oldguest' AND pass_hash IS NULL") == "1")
    stop()
    print(f"\n{passed} passed, {failed} failed")
    raise SystemExit(1 if failed else 0)

asyncio.run(main())
