// app.jsx — root: routing, theme/lang state, context, tweaks
const AppCtx = React.createContext(null);
window.AppCtx = AppCtx;

const HUES = [
  { id: "violet", h: 268, sw: "hsl(268 80% 60%)" },
  { id: "grape", h: 285, sw: "hsl(285 78% 60%)" },
  { id: "indigo", h: 250, sw: "hsl(250 80% 62%)" },
  { id: "blue", h: 232, sw: "hsl(232 80% 60%)" },
  { id: "magenta", h: 315, sw: "hsl(315 75% 60%)" },
];

const TWEAK_DEFAULTS = /*EDITMODE-BEGIN*/{
  "hue": 268,
  "theme": "dark",
  "lang": "ru",
  "blur": 26,
  "radius": 1,
  "particles": true,
  "start": "landing",
  "reduceMotion": false
}/*EDITMODE-END*/;

const DEMO_USER = {
  name: "Alex Renn", username: "alex", email: "alex@cotton-id.io",
  about: "Создаю маленькие, аккуратные продукты для интернета. Кофе, типографика и чистый вход.",
  location: "Алматы, Казахстан", since: "2024", logins: 842, avatar: null,
};

function App() {
  const [tw, setTweak] = useTweaks(TWEAK_DEFAULTS);
  const [route, setRoute] = React.useState(tw.start || "landing");
  const [theme, setThemeState] = React.useState(tw.theme || "dark");
  const [lang, setLangState] = React.useState(tw.lang || "ru");
  const [authed, setAuthed] = React.useState(tw.start === "account" || tw.start === "admin");

  // apply theme + tokens
  React.useEffect(() => { document.documentElement.setAttribute("data-theme", theme); }, [theme]);
  React.useEffect(() => { document.documentElement.setAttribute("lang", lang); }, [lang]);
  React.useEffect(() => { document.documentElement.style.setProperty("--h", tw.hue); }, [tw.hue]);
  React.useEffect(() => { document.documentElement.style.setProperty("--blur", tw.blur + "px"); }, [tw.blur]);
  React.useEffect(() => { document.documentElement.style.setProperty("--rs", tw.radius); }, [tw.radius]);
  React.useEffect(() => {
    document.getElementById("bgStage").style.display = tw.particles ? "block" : "none";
  }, [tw.particles]);
  React.useEffect(() => {
    document.body.classList.toggle("no-anim", !!tw.reduceMotion);
  }, [tw.reduceMotion]);

  // keep theme/lang in sync when changed via tweaks
  React.useEffect(() => { setThemeState(tw.theme); }, [tw.theme]);
  React.useEffect(() => { setLangState(tw.lang); }, [tw.lang]);

  const setTheme = (v) => { setThemeState(v); setTweak("theme", v); };
  const setLang = (v) => { setLangState(v); setTweak("lang", v); };
  const go = (r) => {
    if (r === "account" || r === "admin") setAuthed(true);
    setRoute(r);
    const el = document.scrollingElement || document.documentElement;
    el.scrollTo({ top: 0, behavior: "instant" in window ? "instant" : "auto" });
    window.scrollTo(0, 0);
  };
  const login = () => { setAuthed(true); go("account"); };
  const logout = () => { setAuthed(false); go("landing"); };

  const t = window.I18N[lang];
  const ctx = { t, lang, theme, setTheme, setLang, go, route, user: DEMO_USER, authed, login, logout };

  let Screen;
  if (route === "landing") Screen = window.ScreenLanding;
  else if (route === "login") Screen = window.ScreenLogin;
  else if (route === "signup") Screen = window.ScreenSignup;
  else if (route === "account") Screen = window.ScreenAccount;
  else if (route === "admin") Screen = window.ScreenAdmin;
  else Screen = window.ScreenLanding;

  return (
    <AppCtx.Provider value={ctx}>
      <Screen />
      <TweaksPanel title="Tweaks">
        <TweakSection label={lang === "ru" ? "Внешний вид" : "Appearance"} />
        <HueControl label={lang === "ru" ? "Акцент" : "Accent"} value={tw.hue} onChange={(v) => setTweak("hue", v)} />
        <TweakRadio label={lang === "ru" ? "Тема" : "Theme"} value={theme}
          options={[{ value: "dark", label: lang === "ru" ? "Тёмная" : "Dark" }, { value: "light", label: lang === "ru" ? "Светлая" : "Light" }]}
          onChange={setTheme} />
        <TweakRadio label={lang === "ru" ? "Язык" : "Language"} value={lang}
          options={[{ value: "ru", label: "RU" }, { value: "en", label: "EN" }]} onChange={setLang} />

        <TweakSection label={lang === "ru" ? "Стекло и форма" : "Glass & shape"} />
        <TweakSlider label={lang === "ru" ? "Размытие стекла" : "Glass blur"} value={tw.blur} min={6} max={44} step={1} unit="px" onChange={(v) => setTweak("blur", v)} />
        <TweakSlider label={lang === "ru" ? "Скругление" : "Corner radius"} value={tw.radius} min={0.4} max={1.7} step={0.05} onChange={(v) => setTweak("radius", v)} />

        <TweakSection label={lang === "ru" ? "Фон и движение" : "Background & motion"} />
        <TweakToggle label={lang === "ru" ? "Частицы на фоне" : "Background particles"} value={tw.particles} onChange={(v) => setTweak("particles", v)} />
        <TweakToggle label={lang === "ru" ? "Меньше анимаций" : "Reduce motion"} value={tw.reduceMotion} onChange={(v) => setTweak("reduceMotion", v)} />

        <TweakSection label={lang === "ru" ? "Навигация" : "Navigation"} />
        <TweakSelect label={lang === "ru" ? "Открыть экран" : "Go to screen"} value={route}
          options={[
            { value: "landing", label: lang === "ru" ? "Лендинг" : "Landing" },
            { value: "login", label: lang === "ru" ? "Вход" : "Sign in" },
            { value: "signup", label: lang === "ru" ? "Регистрация" : "Sign up" },
            { value: "account", label: lang === "ru" ? "Аккаунт" : "Account" },
            { value: "admin", label: lang === "ru" ? "Админка" : "Admin" },
          ]}
          onChange={(v) => { setTweak("start", v); go(v); }} />
      </TweaksPanel>
    </AppCtx.Provider>
  );
}

/* custom hue swatch control inside the tweaks panel */
function HueControl({ label, value, onChange }) {
  return (
    <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12, padding: "7px 0" }}>
      <span style={{ fontSize: 13, fontWeight: 600, color: "var(--text-2)" }}>{label}</span>
      <div style={{ display: "flex", gap: 8 }}>
        {HUES.map((o) => (
          <button key={o.id} onClick={() => onChange(o.h)} title={o.id} style={{
            width: 26, height: 26, borderRadius: "50%", background: o.sw, cursor: "pointer",
            border: "2px solid " + (Math.abs(value - o.h) < 1 ? "var(--text)" : "transparent"),
            boxShadow: Math.abs(value - o.h) < 1 ? "0 0 0 2px var(--bg), 0 4px 10px -3px " + o.sw : "0 2px 6px -2px rgba(0,0,0,.4)",
            transition: "all .25s ease", transform: Math.abs(value - o.h) < 1 ? "scale(1.12)" : "none",
          }} />
        ))}
      </div>
    </div>
  );
}

ReactDOM.createRoot(document.getElementById("root")).render(<App />);
