// Retakes the README screenshots that show Cmdhive's own branding (the welcome
// screen's wordmark), from the real page with made-up data:
//   node scripts/screenshots.cjs        -> docs/screenshots/models.png, settings.png
// Needs Playwright, as the UI test does.
const { chromium } = require("playwright");
const fs = require("fs");
const path = require("path");

const fe = path.join(__dirname, "../frontend");
const out = path.join(__dirname, "../docs/screenshots");

async function welcome(page) {
  await page.setContent(fs.readFileSync(path.join(fe, "index.html"), "utf8")
    .replace(/<script[\s\S]*?<\/script>/g, "").replace(/<link[^>]*>/g, ""));
  for (const f of ["style.css", "chat.css", "term.css"]) await page.addStyleTag({ path: path.join(fe, f) });
  for (const f of ["vendor/marked.js", "app.js", "chat.js", "settings.js", "setup.js"]) await page.addScriptTag({ path: path.join(fe, f) });
  await page.evaluate(async () => {
    window.go = { main: { App: {
      History: async () => [], ChatControl: async () => {}, Accounts: async () => [],
      GetSettings: async () => ({ style: "terminal", theme: "campbell", custom: {}, fontSize: 14, chatView: "native", permissions: "ask",
        alwaysOnTop: false, prewarm: true, userName: "", models: {}, onboarded: true, access: "folder", autoUpdate: true,
        autoInstall: false, crossAI: "switch", notify: true, reopenTabs: true, browserForget: false, browserSites: [] }),
      MemoryFolder: async () => "~/.claude/memory", Agents: async () => [], BrowserAvailable: async () => true,
      AppVersion: async () => APP_VERSION, CheckUpdate: async () => ({ available: false }), GetRules: async () => ({ text: "", presets: [] }),
    } } };
    APP_VERSION = "1.9.0";
    const fam = (family, desc, names) => names.map((n, i) => ({ id: `${family}-${n}`.toLowerCase().replace(/[ .]/g, "-"), name: `${family} ${n}`, family, desc }));
    ui.profiles = [
      { id: "claude", name: "Claude", agent: true, chat: true, installed: true, models: [
        ...fam("Fable", "Newest, most capable", ["5.1"]), ...fam("Opus", "Deep reasoning", ["5.5", "5", "4.8"]),
        ...fam("Sonnet", "Fast and capable", ["5.5", "5"]), ...fam("Haiku", "Fastest, cheapest", ["4.5"]) ] },
      { id: "codex", name: "ChatGPT", agent: true, chat: true, installed: true, models: [] },
    ];
    ui.settings = await go.main.App.GetSettings(); // what Settings shows
    // The tab bar, as AIT draws it, and one chat in it.
    const pane = document.createElement("div");
    pane.style.cssText = "position:fixed;inset:38px 0 0;display:flex";
    document.body.append(pane);
    const tab = { id: 1, native: true, profile: "claude", pane, title: "Claude", account: "Claude 1" };
    tabs.set(1, tab); active = 1;
    tab.el = makeTabEl(tab); tab.el.classList.add("active"); $("#tabs").append(tab.el);
    const acct = $("#acct"); acct.hidden = false; acct.querySelector(".name").textContent = "Claude 1";
    createChat(tab);
    const w = tab.chat.welcome;
    w.querySelector(".wb-acct").textContent = "Claude 1";
    w.querySelector(".wb-model").textContent = "Opus 5.5";
    w.querySelector(".wb-folder").textContent = "~\\Projects\\shop-api";
    tab.chat.root.querySelector(".cm-name").textContent = "Opus 5.5";
    document.body.classList.add("ready");
  });
  await page.waitForTimeout(1600); // the wordmark's entrance
}

(async () => {
  const browser = await chromium.launch({ channel: "chrome", headless: true });
  try {
    let page = await browser.newPage({ viewport: { width: 1280, height: 800 } });
    await welcome(page);
    await page.evaluate(() => modelMenu(tabs.get(1)));
    await page.waitForTimeout(400);
    await page.screenshot({ path: path.join(out, "models.png") });
    await page.close();

    page = await browser.newPage({ viewport: { width: 1280, height: 800 } });
    await welcome(page);
    await page.evaluate(() => openSettings());
    await page.waitForTimeout(700);
    await page.screenshot({ path: path.join(out, "settings.png") });
    console.log("models.png, settings.png");
  } finally {
    await browser.close();
  }
})().catch((e) => { console.error(e); process.exit(1); });
