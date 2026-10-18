"""Starts the real server and fetches the site the way a browser does, then reports what is actually sent.
Needs the database from tests/README.md. Run after `npm run build` and a fresh `go build`."""
import gzip, os, re, subprocess, sys, time, urllib.request, urllib.error, signal

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
PORT = 8113
DB = os.environ.get("TEST_DATABASE_URL", "postgres://lobby:lobby@localhost/lobbytest?sslmode=disable")
passed = failed = 0
def check(name, ok, extra=""):
    global passed, failed
    if ok: passed += 1
    else: failed += 1; print("FAIL", name, extra)

def fetch(path, method="GET", headers=None):
    req = urllib.request.Request("http://127.0.0.1:%d%s" % (PORT, path), method=method, headers=headers or {})
    try:
        r = urllib.request.urlopen(req, timeout=10)
        return r.status, {k.lower(): v for k, v in r.headers.items()}, r.read()
    except urllib.error.HTTPError as e:
        return e.code, {k.lower(): v for k, v in e.headers.items()}, e.read()

binary = os.path.join(ROOT, "lobby-test")
env = dict(os.environ, DATABASE_URL=DB, PORT=str(PORT), ADMIN_HANDLE="Boss", ADMIN_PASSWORD="boss password 1", HASH_ITERATIONS="1000")
log = open("/tmp/lobby_site.log", "w")
srv = subprocess.Popen([binary], env=env, stdout=log, stderr=log, cwd=ROOT)
try:
    for _ in range(60):
        try: fetch("/"); break
        except Exception: time.sleep(0.25)
    gz = {"Accept-Encoding": "gzip"}
    st, h, body = fetch("/", headers=gz)
    check("the page is served compressed to browsers that accept it", st == 200 and h.get("content-encoding") == "gzip" and "text/html" in h.get("content-type", ""), h)
    check("the page is checked on every visit", h.get("cache-control") == "no-cache" and "etag" in h)
    html = gzip.decompress(body).decode()
    check("the page is small, it only points at the app", len(html) < 3000, len(html))
    st2, _, b2 = fetch("/", headers={"If-None-Match": h["etag"], **gz})
    check("an unchanged page costs nothing but the answer 'not modified'", st2 == 304 and len(b2) == 0)
    plain = fetch("/")
    check("a client that cannot decompress still gets the page", plain[0] == 200 and "content-encoding" not in plain[1] and len(plain[2]) == len(html.encode()))
    refs = re.findall(r'(?:href|src)="(/assets/[^"]+)"', html)
    check("the page names a stylesheet and an app script", any(r.endswith(".css") for r in refs) and any(r.endswith(".js") for r in refs), refs)
    first = len(body)
    rows = [("/", len(body), len(html.encode()))]
    for r in refs:
        st, h, b = fetch(r, headers=gz)
        check("%s is served compressed, kept for a year and never changes" % r, st == 200 and h.get("content-encoding") == "gzip" and "immutable" in h.get("cache-control", "") and "max-age=31536000" in h.get("cache-control", ""), h)
        rows.append((r, len(b), len(gzip.decompress(b))))
        first += len(b)
    # everything in assets, so the staff files are measured too
    names = sorted(os.listdir(os.path.join(ROOT, "web", "assets")))
    staff = 0
    for n in names:
        if "StaffArea" in n:
            st, h, b = fetch("/assets/" + n, headers=gz)
            check("/assets/%s (staff screens) is served the same way" % n, st == 200 and "immutable" in h.get("cache-control", ""))
            rows.append(("/assets/" + n, len(b), len(gzip.decompress(b)) if h.get("content-encoding") == "gzip" else len(b)))
            staff += len(b)
    check("the only files are the page, the app, its stylesheet and the staff screens", len(names) == 4, names)
    check("nothing else is served", all(fetch(p)[0] == 404 for p in ("/secret", "/assets/", "/assets/nope.js", "/web/index.html", "/index.html")))
    check("HEAD answers with headers only", (lambda r: r[0] == 200 and len(r[2]) == 0)(fetch("/", "HEAD")))
    check("other methods are refused", fetch("/", "POST")[0] == 405)
    print("\nwhat the real server sends (as a browser that accepts gzip receives it)")
    print("  %-44s %12s %12s" % ("file", "sent", "uncompressed"))
    for n, s, r in rows: print("  %-44s %9.1f KB %9.1f KB" % (n, s / 1024, r / 1024))
    print("  %-44s %9.1f KB" % ("first visit, everyone", first / 1024))
    print("  %-44s %9.1f KB" % ("extra for moderators and admins, once", staff / 1024))
    print("  %-44s %9.1f KB" % ("repeat visit (page only, answered 'not modified')", 0.3))
finally:
    srv.send_signal(signal.SIGTERM)
    try: srv.wait(5)
    except Exception: srv.kill()
print("\n%d passed, %d failed" % (passed, failed))
sys.exit(1 if failed else 0)
