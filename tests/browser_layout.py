import sys, json, time
from playwright.sync_api import sync_playwright
import os
HTML = "file://" + os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "web", "index.html"))
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
    pg = ctx.new_page(); pg.goto(HTML); pg.wait_for_timeout(300)
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
print(f"\n{passed} passed, {failed} failed")
raise SystemExit(1 if failed else 0)
