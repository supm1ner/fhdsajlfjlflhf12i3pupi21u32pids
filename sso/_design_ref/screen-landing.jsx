// screen-landing.jsx
function useApp() { return React.useContext(window.AppCtx); }

const DEMO_SERVICES = [
  { name: "Vault", icon: "lock", c: ["#a855f7", "#6d28d9"] },
  { name: "Mailbox", icon: "mail", c: ["#ec4899", "#7c3aed"] },
  { name: "Studio", icon: "sparkle", c: ["#8b5cf6", "#2563eb"] },
  { name: "Cloud", icon: "layers", c: ["#c084fc", "#4f46e5"] },
  { name: "Stream", icon: "activity", c: ["#f472b6", "#9333ea"] },
  { name: "Notes", icon: "doc", c: ["#818cf8", "#9333ea"] },
];

function FeatureCard({ icon, title, desc, delay }) {
  const ref = React.useRef(null);
  return (
    <div ref={ref} className="glass rise" style={{ padding: 28, borderRadius: "var(--r-lg)", animationDelay: `${delay}s`, transition: "transform .5s var(--ease), box-shadow .5s var(--ease)" }}
      onMouseMove={(e) => { const r = ref.current.getBoundingClientRect(); ref.current.style.setProperty("--mx", `${e.clientX - r.left}px`); ref.current.style.setProperty("--my", `${e.clientY - r.top}px`); }}
      onMouseEnter={(e) => { e.currentTarget.style.transform = "translateY(-6px)"; }}
      onMouseLeave={(e) => { e.currentTarget.style.transform = "none"; }}>
      <div style={{ width: 54, height: 54, borderRadius: 17, display: "grid", placeItems: "center", marginBottom: 20,
        background: "linear-gradient(140deg, var(--accent-soft), transparent)", border: "1px solid var(--accent-line)", color: "var(--accent)" }}>
        <Icon name={icon} size={26} />
      </div>
      <h3 style={{ fontSize: 21, fontWeight: 600, marginBottom: 9, color: "var(--text)" }}>{title}</h3>
      <p style={{ fontSize: 15, lineHeight: 1.55, color: "var(--text-3)" }}>{desc}</p>
    </div>
  );
}

function HeroPreview({ user, t }) {
  return (
    <div className="glass rise" style={{ borderRadius: "var(--r-xl)", padding: 0, overflow: "hidden", animationDelay: ".25s", width: "100%", maxWidth: 420 }}>
      {/* banner */}
      <div style={{ height: 116, background: "linear-gradient(120deg, var(--accent-2), var(--accent-strong) 60%, #db2777)", position: "relative" }}>
        <div style={{ position: "absolute", inset: 0, background: "radial-gradient(120% 120% at 80% 0%, rgba(255,255,255,.35), transparent 50%)" }} />
      </div>
      <div style={{ padding: "0 26px 26px" }}>
        <div style={{ marginTop: -42, marginBottom: 14 }}>
          <Avatar name={user.name} src={user.avatar} size={84} ring />
        </div>
        <div className="row" style={{ gap: 9, marginBottom: 4 }}>
          <span style={{ fontSize: 22, fontWeight: 700, color: "var(--text)", whiteSpace: "nowrap" }}>{user.name}</span>
          <Icon name="shield" size={18} fill="var(--accent)" style={{ color: "var(--accent)" }} />
        </div>
        <span style={{ fontSize: 14.5, color: "var(--text-3)" }}>@{user.username} · {user.email}</span>
        <div className="row" style={{ gap: 8, marginTop: 18, flexWrap: "wrap" }}>
          {DEMO_SERVICES.slice(0, 5).map((s, i) => (
            <div key={i} style={{ width: 38, height: 38, borderRadius: 12, display: "grid", placeItems: "center", color: "#fff",
              background: `linear-gradient(140deg, ${s.c[0]}, ${s.c[1]})`, boxShadow: "inset 0 1px 0 rgba(255,255,255,.4)" }}>
              <Icon name={s.icon} size={18} />
            </div>
          ))}
          <div style={{ width: 38, height: 38, borderRadius: 12, display: "grid", placeItems: "center", color: "var(--text-2)", background: "var(--glass-2)", border: "1px solid var(--glass-border)", fontWeight: 700, fontSize: 13 }}>+7</div>
        </div>
      </div>
    </div>
  );
}

function ScreenLanding() {
  const { t, lang, theme, setTheme, setLang, go, user, authed } = useApp();
  const links = [
    { id: "f", to: "landing", label: t.nav_features },
    { id: "s", to: "landing", label: t.nav_services },
    { id: "sec", to: "landing", label: t.nav_security },
  ];
  const stat = (n, l) => (
    <div className="col" style={{ gap: 2 }}>
      <span style={{ fontFamily: "var(--serif)", fontSize: 34, color: "var(--text)", lineHeight: 1 }}>{n}</span>
      <span style={{ fontSize: 13.5, color: "var(--text-3)" }}>{l}</span>
    </div>
  );
  return (
    <div className="screen-enter">
      <NavBar t={t} theme={theme} setTheme={setTheme} lang={lang} setLang={setLang} go={go} user={authed ? user : null} links={links} />

      {/* HERO */}
      <section style={{ maxWidth: 1180, margin: "0 auto", padding: "150px 28px 60px", display: "grid", gridTemplateColumns: "1.1fr .9fr", gap: 50, alignItems: "center" }} className="hero-grid">
        <div className="col" style={{ gap: 26 }}>
          <div className="rise"><Badge dot color="var(--accent)" bg="var(--accent-soft)" style={{ fontSize: 13.5, padding: "7px 14px 7px 11px" }}>{t.hero_badge}</Badge></div>
          <h1 className="rise" style={{ fontSize: "clamp(40px, 5.2vw, 68px)", lineHeight: 1.02, animationDelay: ".05s", letterSpacing: "-.02em" }}>
            {t.hero_title_1}<br />
            <span className="serif ital" style={{ fontWeight: 400, background: "linear-gradient(120deg, var(--accent-2), var(--accent))", WebkitBackgroundClip: "text", backgroundClip: "text", color: "transparent" }}>{t.hero_title_em}</span>
          </h1>
          <p className="rise" style={{ fontSize: 18.5, lineHeight: 1.55, color: "var(--text-2)", maxWidth: 480, animationDelay: ".1s" }}>{t.hero_sub}</p>
          <div className="row rise" style={{ gap: 13, animationDelay: ".15s", flexWrap: "wrap" }}>
            <Button size="lg" iconRight="arrow" onClick={() => go("signup")}>{t.hero_cta}</Button>
            <Button size="lg" variant="glass" onClick={() => go("login")}>{t.hero_cta_2}</Button>
          </div>
          <div className="row rise" style={{ gap: 40, marginTop: 14, animationDelay: ".2s", flexWrap: "wrap" }}>
            {stat("48+", t.hero_stat_1)}
            {stat("2s", t.hero_stat_2)}
            {stat("99.9%", t.hero_stat_3)}
          </div>
        </div>
        <div className="center"><HeroPreview user={user} t={t} /></div>
      </section>

      {/* FEATURES */}
      <section style={{ maxWidth: 1180, margin: "0 auto", padding: "70px 28px" }}>
        <div className="col center" style={{ gap: 14, textAlign: "center", marginBottom: 46 }}>
          <h2 style={{ fontSize: "clamp(30px, 3.6vw, 46px)" }}>{t.feat_title}</h2>
          <p style={{ fontSize: 17.5, color: "var(--text-3)", maxWidth: 560 }}>{t.feat_sub}</p>
        </div>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(250px, 1fr))", gap: 20 }}>
          <FeatureCard icon="bolt" title={t.feat_1_t} desc={t.feat_1_d} delay={0} />
          <FeatureCard icon="shield" title={t.feat_2_t} desc={t.feat_2_d} delay={0.08} />
          <FeatureCard icon="user" title={t.feat_3_t} desc={t.feat_3_d} delay={0.16} />
          <FeatureCard icon="eye" title={t.feat_4_t} desc={t.feat_4_d} delay={0.24} />
        </div>
      </section>

      {/* SERVICES */}
      <section style={{ maxWidth: 1180, margin: "0 auto", padding: "60px 28px" }}>
        <div className="glass" style={{ borderRadius: "var(--r-xl)", padding: "46px 40px" }}>
          <div className="col center" style={{ gap: 12, textAlign: "center", marginBottom: 38 }}>
            <h2 style={{ fontSize: "clamp(26px, 3vw, 38px)" }}>{t.services_title}</h2>
            <p style={{ fontSize: 16.5, color: "var(--text-3)" }}>{t.services_sub}</p>
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(150px, 1fr))", gap: 16 }}>
            {DEMO_SERVICES.map((s, i) => (
              <div key={i} className="row" style={{ gap: 13, padding: "16px 18px", borderRadius: "var(--r)", background: "var(--glass-2)", border: "1px solid var(--glass-border)", transition: "all .4s var(--ease)" }}
                onMouseEnter={(e) => { e.currentTarget.style.transform = "translateY(-4px)"; e.currentTarget.style.background = "var(--glass-3)"; }}
                onMouseLeave={(e) => { e.currentTarget.style.transform = "none"; e.currentTarget.style.background = "var(--glass-2)"; }}>
                <div style={{ width: 42, height: 42, borderRadius: 13, display: "grid", placeItems: "center", color: "#fff", background: `linear-gradient(140deg, ${s.c[0]}, ${s.c[1]})`, boxShadow: "inset 0 1px 0 rgba(255,255,255,.4)" }}>
                  <Icon name={s.icon} size={21} />
                </div>
                <div className="col" style={{ gap: 1 }}>
                  <span style={{ fontSize: 15.5, fontWeight: 600, color: "var(--text)" }}>{s.name}</span>
                  <span style={{ fontSize: 12.5, color: "hsl(150 60% 55%)", fontWeight: 600 }}>● {t.acc_connected}</span>
                </div>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* CTA */}
      <section style={{ maxWidth: 1180, margin: "0 auto", padding: "60px 28px 40px" }}>
        <div className="glass center" style={{ borderRadius: "var(--r-xl)", padding: "70px 40px", textAlign: "center", overflow: "hidden", position: "relative" }}>
          <div style={{ position: "absolute", inset: 0, background: "radial-gradient(80% 120% at 50% 0%, var(--accent-soft), transparent 60%)", pointerEvents: "none" }} />
          <div className="col center" style={{ gap: 22, position: "relative", maxWidth: 560 }}>
            <h2 style={{ fontSize: "clamp(30px, 4vw, 50px)" }}>{t.cta_title}</h2>
            <p style={{ fontSize: 18, color: "var(--text-2)", lineHeight: 1.5 }}>{t.cta_sub}</p>
            <Button size="lg" iconRight="arrow" onClick={() => go("signup")}>{t.hero_cta}</Button>
          </div>
        </div>
      </section>

      {/* FOOTER */}
      <footer style={{ maxWidth: 1180, margin: "0 auto", padding: "30px 28px 50px" }}>
        <div className="row" style={{ justifyContent: "space-between", flexWrap: "wrap", gap: 16, paddingTop: 26, borderTop: "1px solid var(--glass-border)" }}>
          <Logo size={24} onClick={() => go("landing")} />
          <span className="row" style={{ gap: 8, fontSize: 13.5, color: "var(--text-3)" }}><Icon name="shield" size={15} /> {t.foot_made}</span>
          <button onClick={() => go("admin")} style={{ fontSize: 13.5, color: "var(--text-3)", fontWeight: 600 }}>Admin →</button>
        </div>
      </footer>
    </div>
  );
}
window.ScreenLanding = ScreenLanding;
window.DEMO_SERVICES = DEMO_SERVICES;
