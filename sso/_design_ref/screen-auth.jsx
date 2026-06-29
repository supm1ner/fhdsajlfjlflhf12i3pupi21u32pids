// screen-auth.jsx — Login + Sign up
function useAppA() { return React.useContext(window.AppCtx); }

function AuthChrome({ children, t, theme, setTheme, lang, setLang, go }) {
  return (
    <div className="screen-enter" style={{ minHeight: "100vh", display: "grid", placeItems: "center", padding: "92px 22px 40px" }}>
      <div style={{ position: "fixed", top: 18, left: 22, zIndex: 40 }}>
        <button onClick={() => go("landing")} className="row" style={{ gap: 9, height: 44, padding: "0 16px 0 12px", borderRadius: "var(--r-pill)", background: "var(--glass-2)", border: "1px solid var(--glass-border)", color: "var(--text-2)", backdropFilter: "blur(18px)", fontWeight: 600, fontSize: 14.5 }}>
          <Icon name="back" size={18} /> cotton-id
        </button>
      </div>
      <div className="row" style={{ position: "fixed", top: 18, right: 22, zIndex: 40, gap: 9 }}>
        <LangSwitch lang={lang} setLang={setLang} />
        <ThemeSwitch theme={theme} setTheme={setTheme} />
      </div>
      {children}
    </div>
  );
}

function SocialRow({ t }) {
  const socials = [
    { name: "Google", c: "#fff", g: ["#ea4335", "#4285f4"] },
    { name: "Apple", c: "#fff", g: ["#a1a1aa", "#52525b"] },
    { name: "GitHub", c: "#fff", g: ["#a855f7", "#6d28d9"] },
  ];
  return (
    <div className="col" style={{ gap: 16 }}>
      <div className="row" style={{ gap: 14, color: "var(--text-3)", fontSize: 13 }}>
        <div style={{ flex: 1, height: 1, background: "var(--glass-border)" }} />
        {t.or_continue}
        <div style={{ flex: 1, height: 1, background: "var(--glass-border)" }} />
      </div>
      <div className="row" style={{ gap: 10 }}>
        {socials.map((s) => (
          <button key={s.name} className="row" style={{ flex: 1, justifyContent: "center", gap: 9, height: 48, borderRadius: "var(--r-sm)", background: "var(--glass-2)", border: "1px solid var(--glass-border)", color: "var(--text)", fontWeight: 600, fontSize: 14.5, transition: "all .3s var(--ease)" }}
            onMouseEnter={(e) => { e.currentTarget.style.background = "var(--glass-3)"; e.currentTarget.style.transform = "translateY(-2px)"; }}
            onMouseLeave={(e) => { e.currentTarget.style.background = "var(--glass-2)"; e.currentTarget.style.transform = "none"; }}>
            <span style={{ width: 20, height: 20, borderRadius: 6, background: `linear-gradient(140deg, ${s.g[0]}, ${s.g[1]})`, display: "inline-block" }} />
            {s.name}
          </button>
        ))}
      </div>
    </div>
  );
}

function ScreenLogin() {
  const { t, lang, theme, setTheme, setLang, go, login } = useAppA();
  const [email, setEmail] = React.useState("alex@cotton-id.io");
  const [pw, setPw] = React.useState("");
  const [show, setShow] = React.useState(false);
  const [remember, setRemember] = React.useState(true);
  return (
    <AuthChrome t={t} theme={theme} setTheme={setTheme} lang={lang} setLang={setLang} go={go}>
      <div className="glass rise" style={{ width: "100%", maxWidth: 440, borderRadius: "var(--r-xl)", padding: "40px 38px" }}>
        <div className="col center" style={{ gap: 16, marginBottom: 30 }}>
          <Logo size={30} mark label={false} />
          <div className="col center" style={{ gap: 7, textAlign: "center" }}>
            <h1 style={{ fontFamily: "var(--serif)", fontWeight: 400, fontSize: 34, lineHeight: 1 }}>{t.login_title}</h1>
            <p style={{ fontSize: 15.5, color: "var(--text-3)" }}>{t.login_sub}</p>
          </div>
        </div>
        <form className="col" style={{ gap: 18 }} onSubmit={(e) => { e.preventDefault(); login(); }}>
          <Field label={t.f_email} type="email" icon="mail" value={email} onChange={setEmail} />
          <Field label={t.f_password} type={show ? "text" : "password"} icon="lock" value={pw} onChange={setPw}
            right={<button type="button" onClick={() => setShow(!show)} style={{ color: "var(--text-3)", display: "grid" }}><Icon name="eye" size={18} /></button>} />
          <div className="row" style={{ justifyContent: "space-between" }}>
            <label className="row" style={{ gap: 9, cursor: "pointer", fontSize: 14, color: "var(--text-2)", fontWeight: 500 }}>
              <Toggle on={remember} onChange={setRemember} /> {t.remember}
            </label>
            <a style={{ fontSize: 14, color: "var(--accent)", fontWeight: 600, cursor: "pointer" }}>{t.forgot}</a>
          </div>
          <Button size="lg" full type="submit" iconRight="arrow">{t.btn_login}</Button>
          <button type="button" className="row" style={{ justifyContent: "center", gap: 9, height: 50, borderRadius: "var(--r-sm)", color: "var(--text-2)", fontWeight: 600, fontSize: 14.5, border: "1px dashed var(--glass-border-2)" }}
            onClick={login}><Icon name="finger" size={19} style={{ color: "var(--accent)" }} /> {t.passkey}</button>
          <SocialRow t={t} />
        </form>
        <p className="center" style={{ marginTop: 26, fontSize: 14.5, color: "var(--text-3)" }}>
          {t.no_account} <a onClick={() => go("signup")} style={{ color: "var(--accent)", fontWeight: 700, cursor: "pointer", marginLeft: 5 }}>{t.go_signup}</a>
        </p>
      </div>
    </AuthChrome>
  );
}

function strength(pw) {
  let s = 0;
  if (pw.length >= 8) s++;
  if (/[A-Z]/.test(pw) && /[a-z]/.test(pw)) s++;
  if (/\d/.test(pw)) s++;
  if (/[^A-Za-z0-9]/.test(pw)) s++;
  return Math.min(s, 3);
}

function ScreenSignup() {
  const { t, lang, theme, setTheme, setLang, go, login } = useAppA();
  const [name, setName] = React.useState("");
  const [username, setUsername] = React.useState("");
  const [email, setEmail] = React.useState("");
  const [pw, setPw] = React.useState("");
  const [show, setShow] = React.useState(false);
  const s = strength(pw);
  const sLabels = [t.pw_weak, t.pw_weak, t.pw_ok, t.pw_strong];
  const sColors = ["hsl(350 80% 60%)", "hsl(350 80% 60%)", "hsl(40 90% 58%)", "hsl(150 65% 48%)"];
  return (
    <AuthChrome t={t} theme={theme} setTheme={setTheme} lang={lang} setLang={setLang} go={go}>
      <div className="glass rise" style={{ width: "100%", maxWidth: 460, borderRadius: "var(--r-xl)", padding: "40px 38px" }}>
        <div className="col center" style={{ gap: 16, marginBottom: 28 }}>
          <Logo size={30} mark label={false} />
          <div className="col center" style={{ gap: 7, textAlign: "center" }}>
            <h1 style={{ fontFamily: "var(--serif)", fontWeight: 400, fontSize: 32, lineHeight: 1 }}>{t.signup_title}</h1>
            <p style={{ fontSize: 15.5, color: "var(--text-3)" }}>{t.signup_sub}</p>
          </div>
        </div>
        <form className="col" style={{ gap: 16 }} onSubmit={(e) => { e.preventDefault(); login(); }}>
          <div className="row" style={{ gap: 12 }}>
            <div style={{ flex: 1 }}><Field label={t.f_name} icon="user" value={name} onChange={setName} /></div>
            <div style={{ flex: 1 }}><Field label={t.f_username} value={username} onChange={setUsername} /></div>
          </div>
          <Field label={t.f_email} type="email" icon="mail" value={email} onChange={setEmail} />
          <Field label={t.f_password} type={show ? "text" : "password"} icon="lock" value={pw} onChange={setPw}
            right={<button type="button" onClick={() => setShow(!show)} style={{ color: "var(--text-3)", display: "grid" }}><Icon name="eye" size={18} /></button>} />
          {pw && (
            <div className="col" style={{ gap: 7 }}>
              <div className="row" style={{ gap: 6 }}>
                {[0, 1, 2].map((i) => (
                  <div key={i} style={{ flex: 1, height: 5, borderRadius: 99, background: i < s ? sColors[s] : "var(--glass-3)", transition: "all .4s var(--ease)" }} />
                ))}
              </div>
              <span style={{ fontSize: 12.5, color: sColors[s], fontWeight: 600, paddingLeft: 2 }}>{sLabels[s]}</span>
            </div>
          )}
          <Button size="lg" full type="submit" iconRight="arrow">{t.btn_signup}</Button>
          <SocialRow t={t} />
          <p style={{ fontSize: 12.5, color: "var(--text-3)", textAlign: "center", lineHeight: 1.5 }}>{t.agree}</p>
        </form>
        <p className="center" style={{ marginTop: 22, fontSize: 14.5, color: "var(--text-3)" }}>
          {t.have_account} <a onClick={() => go("login")} style={{ color: "var(--accent)", fontWeight: 700, cursor: "pointer", marginLeft: 5 }}>{t.go_login}</a>
        </p>
      </div>
    </AuthChrome>
  );
}

window.ScreenLogin = ScreenLogin;
window.ScreenSignup = ScreenSignup;
