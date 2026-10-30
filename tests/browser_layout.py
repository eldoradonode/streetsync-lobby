import sys, json, time
from playwright.sync_api import sync_playwright
import os, threading, http.server, socketserver, urllib.parse

# The real production files, served the way the Go server serves them (page checked every time, assets kept for a year),
# and counting what a browser asks for. Modules cannot be loaded from file://, so the test needs a real server.
WEB = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "web"))
HITS = {}
class Site(http.server.SimpleHTTPRequestHandler):
    def __init__(self, *a, **k): super().__init__(*a, directory=WEB, **k)
    def log_message(self, *a): pass
    def do_GET(self):
        path = urllib.parse.urlparse(self.path).path
        HITS[path] = HITS.get(path, 0) + 1
        self._err = False
        if os.environ.get("BLOCK") and os.environ["BLOCK"] in path:
            self._err = True; self.send_error(503); return
        super().do_GET()
    def end_headers(self):
        if getattr(self, "_err", False): self.send_header("Cache-Control", "no-store")   # errors are never kept
        else: self.send_header("Cache-Control", "public, max-age=31536000, immutable" if self.path.startswith("/assets/") else "no-cache")
        super().end_headers()
class Quiet(socketserver.ThreadingMixIn, http.server.HTTPServer): daemon_threads = True
SERVER = Quiet(("127.0.0.1", 0), Site)
threading.Thread(target=SERVER.serve_forever, daemon=True).start()
HTML = "http://127.0.0.1:%d/" % SERVER.server_address[1]
INIT = """
window.__S=[];
window.WebSocket=class{constructor(){this.readyState=0;this.sent=[];window.__S.push(this);setTimeout(()=>{this.readyState=1;this.onopen&&this.onopen()},20)}
send(d){this.sent.push(JSON.parse(d))}close(){this.readyState=3}};
window.__recv=o=>{const s=window.__S[window.__S.length-1];s.onmessage&&s.onmessage({data:JSON.stringify(o)})};
"""
NOW = int(time.time()*1000)
ROOMS = [
 {"s":"lobby","n":"General Chat","c":"General","g":["Vibes","Daily"],"u":128,"h":4},
 {"s":"abuja","n":"Abuja","c":"City","g":["Local","Meetups"],"u":54,"h":3},
 {"s":"football","n":"Sport","c":"Sport","g":["Football","Basketball","Athletics"],"u":31,"h":2},
 {"s":"music","n":"Music Heads","c":"Music","g":["Afrobeats","Hip hop","Playlists"],"u":12,"h":1},
 {"s":"tech","n":"Tech Hustle","c":"Tech","g":["Tech","Business"],"u":0,"h":0},
 {"s":"elite","n":"Elite Lounge","c":"Members","g":["Elite","Private"],"u":0,"h":0,"t":3},
 {"s":"free-aaaa","n":"Startup","c":"Community","g":["YC","Africa"],"u":6,"h":2,"k":"public","o":"Eldorado","f":2},
 {"s":"paid-bbbb","n":"Night Owls","c":"Community","g":["Gist"],"u":3,"h":1,"k":"public","o":"Amaka"},
 {"s":"mine-cccc","n":"My Den","c":"Community","g":[],"u":1,"h":0,"k":"private","o":"Mike","w":True,"y":True},
]
HIST = [
 {"i":1,"h":"Amaka","b":"Anyone watching the match tonight?","ts":NOW-600000,"r":2,"av":"f3"},
 {"i":2,"h":"Tunde","b":"Always. Kickoff is at 8, come through","ts":NOW-540000,"r":1,"av":"m2"},
 {"i":3,"h":"Zainab","b":"Joining from Abuja, what did I miss? @Mike you around","ts":NOW-400000,"r":3,"av":"f1"},
 {"i":4,"h":"Boss","b":"Keep it friendly please","ts":NOW-300000,"r":0,"o":"admin"},
 {"i":5,"h":"Mike","b":"Right here. Reply test","ts":NOW-120000,"r":1,"q":1,"qh":"Amaka","qb":"Anyone watching the match tonight?"},
]
def boot(p, w, h, mobile=False):
    b = p.chromium.launch()
    ctx = b.new_context(viewport={"width":w,"height":h}, is_mobile=mobile, has_touch=mobile, device_scale_factor=1)
    ctx.add_init_script(INIT + "localStorage.setItem('lobby.k','tok');")
    pg = ctx.new_page(); pg.on('pageerror', lambda e: ERRORS.append(str(e)[:200])); pg.goto(HTML); pg.wait_for_timeout(300)
    return b, pg
def recv(pg, o): pg.evaluate("o=>window.__recv(o)", o); pg.wait_for_timeout(80)
def signed_in(pg):
    recv(pg, {"t":"ok","h":"Mike","k":"tok","p":420,"r":1,"x":2000,"s":3,"g":True,"c":120,"v":"m1"})
    recv(pg, {"t":"rooms","r":ROOMS})
def s_hub(pg): signed_in(pg)
def s_rooms(pg): signed_in(pg); pg.click('#tabRooms')
def s_room(pg):
    signed_in(pg); pg.click('.room[data-slug="lobby"]')
    recv(pg, {"t":"joined","r":"lobby","n":"General Chat"}); recv(pg, {"t":"hist","m":HIST}); recv(pg, {"t":"n","n":128})
def s_hostroom(pg):
    signed_in(pg); pg.click('.room[data-slug="mine-cccc"]')
    recv(pg, {"t":"joined","r":"mine-cccc","n":"My Den","k":"private","o":"Mike","y":True,"i":"ABCD2345","w":True,"f":0}); recv(pg, {"t":"hist","m":HIST[:3]}); recv(pg, {"t":"n","n":3})
def s_inbox(pg):
    signed_in(pg); pg.click('#tabInbox')
    recv(pg, {"t":"inbox","c":[{"h":"Ann","i":5,"m":False,"b":"hello there","ts":NOW,"u":2},{"h":"Bob","i":4,"m":True,"b":"see you later","ts":NOW-90000000,"u":0}],"f":["Cat"],"q":["Dan"],"p":[],"x":[]})
def s_thread(pg):
    signed_in(pg); pg.click('#tabInbox')
    recv(pg, {"t":"inbox","c":[{"h":"Ann","i":5,"m":False,"b":"hello there","ts":NOW,"u":2}],"f":["Cat"],"q":[],"p":[],"x":[]})
    pg.click('#chatList li button')
    recv(pg, {"t":"thread","h":"Ann","s":"friend","c":0,"m":[{"i":5,"f":"Ann","w":"Mike","b":"hello there","ts":NOW-60000},{"i":6,"f":"Mike","w":"Ann","b":"hi Ann, how far?","ts":NOW-30000}]})
def s_create(pg): signed_in(pg); pg.click('#tabRooms'); pg.click('#rdCreate')
def s_join(pg): pg.evaluate("localStorage.removeItem('lobby.k')"); pg.reload(); pg.wait_for_timeout(300)
def s_profile(pg): signed_in(pg); pg.click('#tabProfile'); recv(pg, {"t":"wallet","b":120,"l":[]})

passed = failed = 0
ERRORS = []
def check(name, ok, extra=""):
    global passed, failed
    if ok: passed += 1
    else: failed += 1; print("FAIL", name, extra)

def box(pg, sel):
    return pg.evaluate("s=>{const b=document.querySelector(s).getBoundingClientRect();return [b.left,b.top,b.right,b.bottom]}", sel)
def overflow(pg): return pg.evaluate("document.documentElement.scrollWidth > innerWidth")

with sync_playwright() as p:
    # phone
    b, pg = boot(p, 390, 844, True)
    s_hub(pg)
    check("phone: bottom tabs are on screen", box(pg, "#tabs")[3] <= 844 and box(pg, "#tabs")[1] < 844, box(pg, "#tabs"))
    check("phone: the page itself does not scroll", pg.evaluate("document.documentElement.scrollHeight") == 844)
    check("phone: no sideways overflow", not overflow(pg))
    pg.click("#tabRooms"); pg.wait_for_timeout(100)
    check("phone: rooms directory scrolls inside itself", pg.evaluate("document.documentElement.scrollHeight") == 844 and pg.evaluate("document.querySelector('#roomsDir').scrollHeight > document.querySelector('#roomsDir').clientHeight"))
    check("phone: no sideways overflow in rooms", not overflow(pg))
    pg.click("#rdCreate"); pg.wait_for_timeout(100)
    check("phone: create dialog fits the screen", box(pg, "#createRoom")[3] <= 844 and box(pg, "#createRoom")[0] >= 0)
    pg.click("#createRoom .row .ghost")
    pg.click("#tabHub"); pg.click('.room[data-slug="lobby"]')
    recv(pg, {"t":"joined","r":"lobby","n":"General Chat"}); recv(pg, {"t":"hist","m":HIST}); recv(pg, {"t":"n","n":3})
    check("phone: composer sits at the bottom edge", abs(box(pg, "#send")[3] - 844) < 2, box(pg, "#send"))
    pg.set_viewport_size({"width": 390, "height": 480}); pg.wait_for_timeout(250)
    check("phone: composer stays visible when the keyboard shrinks the screen", abs(box(pg, "#send")[3] - 480) < 2 and pg.evaluate("scrollY") == 0, box(pg, "#send"))
    b.close()
    # desktop
    b, pg = boot(p, 1440, 900, False)
    s_hub(pg)
    sb = box(pg, "#tabs")
    check("desktop: sidebar on the left, full height", sb[0] == 0 and sb[2] > 200 and sb[2] < 280 and sb[3] >= 899, sb)
    cards = pg.evaluate("[...document.querySelectorAll('#roomList .room')].map(r=>Math.round(r.getBoundingClientRect().top))")
    check("desktop: official rooms form a grid, several per row", len(set(cards[:4])) == 1, cards)
    check("desktop: no sideways overflow", not overflow(pg))
    pg.click("#tabInbox"); recv(pg, {"t":"inbox","c":[],"f":[],"q":[],"p":[],"x":[]})
    t, f = box(pg, "#inboxTabs"), box(pg, "#chatList")
    check("desktop: inbox tabs and list line up", abs(t[0] - f[0]) < 2 and abs(t[2] - f[2]) < 2, (t, f))
    b.close()
# ---- step 13 screens
def staff_ok(role):
    return {"t":"ok","h":"Mike","k":"tok","p":0,"r":0,"x":0,"s":0,"g":True,"c":120,"o":role,"bio":"Abuja builder."}
USERS = {"t":"ausers","q":"","u":[{"h":"Amaka","o":"user","p":820,"r":1,"j":NOW-86400000*10,"g":True,"on":True,"c":45,"ls":NOW},{"h":"Tunde","o":"user","p":120,"r":0,"j":NOW,"g":False,"on":False,"mu":25,"c":3,"ls":NOW},{"h":"Kofi","o":"mod","p":0,"r":0,"j":NOW,"g":True,"on":False,"c":0,"ls":NOW}]}
with sync_playwright() as p:
    for (w, h, mobile, name) in ((390, 844, True, "phone"), (1440, 900, False, "desktop")):
        b, pg = boot(p, w, h, mobile)
        recv(pg, staff_ok("admin")); recv(pg, {"t":"rooms","r":ROOMS})
        pg.click("#tabProfile"); recv(pg, {"t":"wallet","c":120,"l":[]}); recv(pg, {"t":"uprofile","h":"Mike","j":NOW-86400000*40})
        check(name + ": profile has no sideways overflow", not overflow(pg))
        check(name + ": the page does not scroll as a whole", pg.evaluate("document.documentElement.scrollHeight") <= h + 1)
        pg.click("#tabWallet"); recv(pg, {"t":"wallet","c":120,"l":[{"i":1,"k":"daily","a":2,"n":"","ts":NOW}]})
        check(name + ": wallet has no sideways overflow", not overflow(pg))
        if mobile:
            pg.click("#tabProfile"); pg.click("#staffAdmin")
        else:
            pg.click("#tabAdmin")
        recv(pg, {"t":"astats","on":4,"rep":1,"mu":0,"ki":0,"ro":10,"rc":2,"ir":3,"us":50,"pw":40,"n1":1,"n7":5,"a1":9,"mo":1,"ad":1,"cr":900,"m1":30,"md":[{"d":"2026-10-1%d" % i,"n":i*3} for i in range(7)],"ud":[{"d":"2026-10-1%d" % i,"n":i} for i in range(7)]})
        check(name + ": admin console is themed apart from the app", pg.evaluate("getComputedStyle(document.querySelector('#shell')).getPropertyValue('--accent').trim().toUpperCase()") == "#FB7185")
        check(name + ": admin overview has no sideways overflow", not overflow(pg))
        tabs_vis = pg.evaluate("getComputedStyle(document.querySelector('.areatabs')).display !== 'none'")
        nav_vis = pg.evaluate("getComputedStyle(document.querySelector('#tabs')).display !== 'none'")
        check(name + ": " + ("section tabs on top, no bottom bar" if mobile else "section sidebar, no top tab strip"), (tabs_vis and not nav_vis) if mobile else (nav_vis and not tabs_vis))
        if mobile: pg.click('.areatabs [role=tab]:has-text("Users")')
        else: pg.click("#sec-users")
        recv(pg, USERS)
        check(name + ": user cards have no sideways overflow", not overflow(pg))
        card = box(pg, '#staffUsers [data-handle="Tunde"]')
        check(name + ": user cards stay inside the screen", card[0] >= 0 and card[2] <= w + 1, card)
        right = pg.evaluate("[...document.querySelectorAll('#staffUsers button, #staffUsers select, #staffUsers input')].reduce((m,e)=>Math.max(m,e.getBoundingClientRect().right),0)")
        check(name + ": no action button spills off the right edge", right <= w + 1, right)
        b.close()
    # moderator colours
    b, pg = boot(p, 1440, 900, False)
    recv(pg, staff_ok("mod")); recv(pg, {"t":"rooms","r":ROOMS}); pg.click("#tabMod")
    check("desktop: moderator dashboard is themed teal", pg.evaluate("getComputedStyle(document.querySelector('#shell')).getPropertyValue('--accent').trim().toUpperCase()") == "#2DD4BF")
    check("desktop: moderators get no Moderators or Audit log section", pg.evaluate("!document.querySelector('#sec-moderators') && !document.querySelector('#sec-log')"))
    b.close()
    # ordinary users: normal theme, no staff area
    b, pg = boot(p, 1440, 900, False); signed_in(pg)
    check("desktop: members see no staff links", pg.evaluate("!document.querySelector('#staffNav') && !document.querySelector('#tabAdmin')"))
    check("desktop: members have the normal cyan accent", pg.evaluate("getComputedStyle(document.documentElement).getPropertyValue('--accent').trim().toUpperCase()") == "#00D9E8")
    b.close()
# ---- step 14: lazy staff screens, caching, two pane messages, notifications and settings
CHATS = [{"h":"ann","b":"hello there","ts":NOW,"m":False,"u":2},{"h":"bob","b":"see you later","ts":NOW-5000,"m":True,"u":0},{"h":"cleo","b":"match tonight?","ts":NOW-9000,"m":False,"u":0}]
INBOX = {"t":"inbox","c":CHATS,"f":[],"q":[],"p":[],"x":[]}
NOTES = {"t":"notes","c":2,"l":[{"i":3,"k":"dm","a":"ann","n":3,"b":"a very long message that goes on and on and on to see whether the row wraps instead of pushing the page sideways","ts":NOW,"u":1},{"i":2,"k":"tag","a":"bob","n":1,"r":"lobby","rn":"General Chat","b":"hey @Mike","ts":NOW-60000,"u":1},{"i":1,"k":"tip","a":"cleo","n":1,"b":"Pizza","m":5,"r":"lobby","rn":"General Chat","ts":NOW-90000}]}
def chunk_hits(): return {k: v for k, v in HITS.items() if "StaffArea" in k}
def asset_hits(): return {k: v for k, v in HITS.items() if k.startswith("/assets/app-") or k.startswith("/assets/main-")}

with sync_playwright() as p:
    # a member never downloads the staff screens
    HITS.clear()
    b, pg = boot(p, 1440, 900, False); signed_in(pg)
    for tab in ("#tabRooms", "#tabInbox", "#tabWallet", "#tabProfile", "#tabSettings", "#bell", "#tabHub"):
        pg.click(tab); pg.wait_for_timeout(80)
    check("a member visits every page and never fetches the staff screens", not chunk_hits(), chunk_hits())
    check("the page, the app and its stylesheet are each fetched once", HITS.get("/") == 1 and all(v == 1 for v in asset_hits().values()) and len(asset_hits()) == 2, (HITS.get("/"), asset_hits()))
    b.close()

    # an admin: fetched on demand, once
    HITS.clear()
    b, pg = boot(p, 1440, 900, False)
    recv(pg, staff_ok("admin")); recv(pg, {"t":"rooms","r":ROOMS})
    check("an admin who has not opened a staff page has not downloaded it either", not chunk_hits())
    pg.hover("#tabAdmin"); pg.wait_for_timeout(400)
    check("pointing at the link fetches the staff screens ahead of the click", len([k for k in chunk_hits() if k.endswith(".js")]) == 1, chunk_hits())
    pg.click("#tabAdmin"); recv(pg, {"t":"astats","on":4,"rep":1,"mu":0,"ki":0,"ro":10,"rc":2,"ir":3,"us":50,"pw":40,"n1":1,"n7":5,"a1":9,"mo":1,"ad":1,"cr":900,"m1":30,"md":[{"d":"2026-10-1%d" % i,"n":i} for i in range(7)],"ud":[{"d":"2026-10-1%d" % i,"n":i} for i in range(7)]})
    check("the console renders from the fetched file, with its own styles", pg.evaluate("!!document.querySelector('#staffArea .statgrid') && getComputedStyle(document.querySelector('.statgrid')).display === 'grid'"))
    for sec in ("users", "moderators", "rooms", "log", "overview"): pg.click("#sec-" + sec)
    pg.click("#areaExit"); pg.hover("#tabAdmin"); pg.click("#tabAdmin")
    check("leaving and coming back never fetches it again", all(v == 1 for v in chunk_hits().values()), chunk_hits())
    before = dict(HITS)
    pg.reload(); pg.wait_for_timeout(400)
    after_assets = {k: v for k, v in HITS.items() if k.startswith("/assets/")}
    check("a reload revalidates only the page, the files with hashed names come from the browser's own copy", HITS.get("/") == 2 and after_assets == {k: v for k, v in before.items() if k.startswith("/assets/")}, (HITS.get("/"), after_assets))
    b.close()

    # the staff file cannot be fetched
    os.environ["BLOCK"] = "StaffArea"; HITS.clear()
    b, pg = boot(p, 1440, 900, False)
    recv(pg, staff_ok("mod")); recv(pg, {"t":"rooms","r":ROOMS})
    pg.click("#tabMod"); pg.wait_for_timeout(500)
    check("if the staff file cannot be loaded the page says so and offers a retry", pg.evaluate("!!document.querySelector('#staffFail') && !!document.querySelector('#staffRetry')"))
    check("and the rest of the app is untouched", pg.evaluate("!!document.querySelector('#tabs') && !!document.querySelector('#bell')"))
    check("and it is tried only once, not over and over", len(chunk_hits()) <= 2 and all(v == 1 for v in chunk_hits().values()), chunk_hits())
    os.environ.pop("BLOCK"); pg.click("#staffRetry"); pg.wait_for_timeout(800)
    recv(pg, staff_ok("mod")); recv(pg, {"t":"rooms","r":ROOMS}); pg.click("#tabMod"); pg.wait_for_timeout(500)
    recv(pg, {"t":"astats","on":1,"rep":0,"mu":0,"ki":0,"ro":10,"rc":0,"ir":0})
    check("after reloading with a working connection it loads", pg.evaluate("!!document.querySelector('#staffArea .statgrid')"))
    b.close()

    # two pane messages, desktop
    b, pg = boot(p, 1440, 900, False); signed_in(pg)
    pg.click("#tabInbox"); recv(pg, INBOX)
    li, em = box(pg, "#inbox"), box(pg, "#threadEmpty")
    check("desktop: the list is on the left and the empty pane fills the rest", li[0] < 300 and li[2] <= em[0] + 1 and em[2] >= 1430 and em[1] < 100, (li, em))
    check("desktop: the empty state is centred text", pg.evaluate("document.querySelector('#threadEmpty').textContent.includes('Select a conversation')"))
    pg.click("#chatList li:nth-child(1) .chatrow"); recv(pg, {"t":"thread","h":"ann","s":"friend","c":0,"m":[{"i":1,"f":"ann","w":"Mike","b":"hello there","ts":NOW}]})
    li, th = box(pg, "#inbox"), box(pg, "#thread")
    check("desktop: the conversation opens beside the list", li[2] <= th[0] + 1 and th[2] >= 1430 and pg.evaluate("document.querySelector('#threadEmpty').hidden"), (li, th))
    ts = box(pg, "#tsend")
    check("desktop: the message box sits at the bottom of the right pane", abs(ts[3] - 900) < 2 and ts[0] >= th[0] - 1, ts)
    check("desktop: both panes scroll on their own, the page does not", pg.evaluate("document.documentElement.scrollHeight") <= 901 and not overflow(pg))
    recv(pg, {"t":"dm","i":2,"f":"cleo","w":"Mike","b":"new one","ts":NOW+1000})
    check("desktop: a message from someone else lands in the list while a conversation stays open", pg.evaluate("document.querySelector('#chatList li b').textContent") == "@cleo" and pg.evaluate("!document.querySelector('#tlog').textContent.includes('new one')"))
    check("desktop: settings is in the sidebar", pg.evaluate("!!document.querySelector('#tabSettings') && getComputedStyle(document.querySelector('#tabSettings')).display !== 'none'"))
    b.close()

    # phone: list first, then the conversation, keyboard friendly
    b, pg = boot(p, 390, 844, True); signed_in(pg)
    pg.click("#tabInbox"); recv(pg, INBOX)
    check("phone: the list is alone on screen", pg.evaluate("document.querySelector('#threadEmpty').offsetParent === null") and not overflow(pg))
    pg.click("#chatList li:nth-child(1) .chatrow"); recv(pg, {"t":"thread","h":"ann","s":"friend","c":0,"m":[{"i":1,"f":"ann","w":"Mike","b":"hello there","ts":NOW}]})
    th = box(pg, "#thread")
    check("phone: the conversation fills the screen", th[0] <= 1 and th[2] >= 389 and pg.evaluate("document.querySelector('#inbox').offsetParent === null"), th)
    check("phone: there is a back button", pg.evaluate("!document.querySelector('#back').hidden") and box(pg, "#back")[2] - box(pg, "#back")[0] >= 36)
    pg.set_viewport_size({"width": 390, "height": 480}); pg.wait_for_timeout(250)
    check("phone: the message box stays above a keyboard sized gap", abs(box(pg, "#tsend")[3] - 480) < 2 and pg.evaluate("scrollY") == 0, box(pg, "#tsend"))
    pg.set_viewport_size({"width": 390, "height": 844}); pg.wait_for_timeout(200)
    pg.click("#back"); recv(pg, INBOX)
    check("phone: back returns to the list", pg.evaluate("document.querySelector('#inbox').offsetParent !== null"))
    b.close()

    # phone: notifications and settings
    b, pg = boot(p, 390, 844, True); signed_in(pg)
    recv(pg, {"t":"nc","c":2})
    bell = box(pg, "#bell")
    check("phone: the bell is in the top bar, large enough to tap, with its badge", bell[1] >= 0 and bell[3] <= 70 and bell[2] - bell[0] >= 40 and pg.evaluate("!!document.querySelector('#bellBadge')"), bell)
    pg.click("#bell"); recv(pg, NOTES)
    check("phone: notifications do not overflow, even with long text", not overflow(pg))
    row = box(pg, "#noteList li:nth-child(1) .noterow")
    check("phone: each row is a comfortable touch target", row[3] - row[1] >= 56 and row[0] >= 0 and row[2] <= 391, row)
    pg.click("#tabProfile"); pg.wait_for_timeout(80)
    check("phone: there is no Settings tab in the bottom bar, the five tabs are the only ones", pg.evaluate("[...document.querySelectorAll('#tabs button')].filter(b => b.offsetParent !== null).length") == 5)
    check("phone: Settings is reached from Profile", pg.evaluate("document.querySelector('#profSettings').offsetParent !== null"))
    b.close()

    # settings page, both sizes
    for (w, h, mobile, name) in ((390, 844, True, "phone"), (1440, 900, False, "desktop")):
        b, pg = boot(p, w, h, mobile); signed_in(pg)
        recv(pg, {"t":"pf","nd":True,"nk":False,"nt":True,"nf":True,"dp":"everyone","so":True})
        if mobile:
            pg.click("#tabProfile"); pg.click("#profSettings")
        else:
            pg.click("#tabSettings")
        shown = pg.evaluate("!document.querySelector('#settingsPage').hidden")
        check(name + ": the Settings page can be opened", shown)
        if shown:
            check(name + ": settings has no sideways overflow", not overflow(pg))
            sw = box(pg, "#setNotes .switchrow:nth-of-type(1)")
            check(name + ": each switch row is a comfortable target", sw[3] - sw[1] >= 44 and sw[2] <= w + 1, sw)
            knob = box(pg, "#sw-nd")
            check(name + ": the switch itself is drawn as a switch", 40 <= knob[2] - knob[0] <= 60 and 22 <= knob[3] - knob[1] <= 34, knob)
            check(name + ": a switch reflects the saved value", pg.evaluate("document.querySelector('#sw-nd').checked && !document.querySelector('#sw-nk').checked"))
        b.close()

check("no page threw a JavaScript error during any of these flows", not ERRORS, ERRORS[:3])
print(f"\n{passed} passed, {failed} failed")
raise SystemExit(1 if failed else 0)
