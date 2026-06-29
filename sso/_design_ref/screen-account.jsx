// screen-account.jsx — Account management
function useAppAcc() { return React.useContext(window.AppCtx); }

function SectionTitle({ title, sub }) {
  return (
    <div className="col" style={{ gap: 4, marginBottom: 4 }}>
      <h3 style={{ fontSize: 20, fontWeight: 600 }}>{title}</h3>
      {sub && <p style={{ fontSize: 14, color: "var(--text-3)" }}>{sub}</p>}
    </div>
  );
}

function GlassTextarea({ value, onChange, placeholder }) {
  return (
    <textarea value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} rows={3}
      style={{ width: "100%", resize: "vertical", padding: "14px 16px", borderRadius: "var(--r-sm)", background: "var(--field)", border: "1.5px solid var(--glass-border)", color: "var(--text)", fontSize: 15.5, lineHeight: 1.5, outline: "none", boxShadow: "inset 0 1px 0 var(--glass-hi)" }} />
  );
}

function ProfileTab({ t, user }) {
  const [about, setAbout] = React.useState(user.about);
  const [loc, setLoc] = React.useState(user.location);
  const [name, setName] = React.useState(user.name);
  const [un, setUn] = React.useState(user.username);
  return (
    <div style={{ display: "grid", gridTemplateColumns: "1.5fr 1fr", gap: 20 }} className="acc-grid">
      <Panel pad={28} className="col" style={{ gap: 20 }}>
        <SectionTitle title={t.acc_profile_title} />
        <div className="row" style={{ gap: 14 }}>
          <div style={{ flex: 1 }}><Field label={t.f_name} value={name} onChange={setName} icon="user" /></div>
          <div style={{ flex: 1 }}><Field label={t.f_username} value={un} onChange={setUn} /></div>
        </div>
        <Field label={t.f_email} value={user.email} onChange={() => {}} icon="mail" />
        <label className="col" style={{ gap: 7 }}>
          <span style={{ fontSize: 13.5, fontWeight: 600, color: "var(--text-2)", paddingLeft: 2 }}>{t.acc_about}</span>
          <GlassTextarea value={about} onChange={setAbout} placeholder={t.acc_about_ph} />
        </label>
        <Field label={t.acc_loc} value={loc} onChange={setLoc} icon="globe" />
        <div className="row" style={{ gap: 12, marginTop: 4 }}>
          <Button icon="check">{t.acc_save}</Button>
          <Button variant="ghost">{t.acc_cancel}</Button>
        </div>
      </Panel>
      <div className="col" style={{ gap: 20 }}>
        <Panel pad={24} className="col" style={{ gap: 16 }}>
          <SectionTitle title={t.acc_services_title} />
          {window.DEMO_SERVICES.slice(0, 4).map((s, i) => (
            <div key={i} className="row" style={{ gap: 12 }}>
              <div style={{ width: 38, height: 38, borderRadius: 12, display: "grid", placeItems: "center", color: "#fff", background: `linear-gradient(140deg, ${s.c[0]}, ${s.c[1]})` }}><Icon name={s.icon} size={18} /></div>
              <span style={{ flex: 1, fontSize: 15, fontWeight: 600 }}>{s.name}</span>
              <Icon name="check" size={18} style={{ color: "hsl(150 60% 50%)" }} />
            </div>
          ))}
        </Panel>
        <Panel pad={24} className="col" style={{ gap: 14 }}>
          <div className="row" style={{ gap: 12 }}>
            <div style={{ width: 44, height: 44, borderRadius: 13, display: "grid", placeItems: "center", background: "var(--accent-soft)", color: "var(--accent)", border: "1px solid var(--accent-line)" }}><Icon name="shield" size={22} /></div>
            <div className="col" style={{ gap: 1 }}>
              <span style={{ fontSize: 15.5, fontWeight: 700 }}>cotton-id Secure</span>
              <span style={{ fontSize: 13, color: "var(--text-3)" }}>2FA · Passkey · {user.logins} logins</span>
            </div>
          </div>
        </Panel>
      </div>
    </div>
  );
}

function SecurityTab({ t, user }) {
  const [twofa, setTwofa] = React.useState(true);
  const [pk, setPk] = React.useState(true);
  const sessions = [
    { dev: "MacBook Pro · Safari", loc: "Алматы, KZ", icon: "monitor", cur: true },
    { dev: "iPhone 15 · cotton-id", loc: "Алматы, KZ", icon: "phone", cur: false },
    { dev: "Chrome · Windows", loc: "Астана, KZ", icon: "monitor", cur: false },
  ];
  return (
    <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 20 }} className="acc-grid">
      <Panel pad={26} className="col" style={{ gap: 8 }}>
        <SectionTitle title={t.acc_security_title} />
        <div style={{ height: 8 }} />
        <ListRow icon="lock" accent title={t.acc_pw_title} sub={t.acc_pw_sub} right={<Button size="sm" variant="glass">{t.acc_edit}</Button>} />
        <div style={{ height: 1, background: "var(--glass-border)", margin: "4px 0" }} />
        <ListRow icon="shield" accent title={t.acc_2fa_title} sub={t.acc_2fa_sub} right={<Toggle on={twofa} onChange={setTwofa} />} />
        <div style={{ height: 1, background: "var(--glass-border)", margin: "4px 0" }} />
        <ListRow icon="finger" accent title={t.acc_passkey_title} sub={t.acc_passkey_sub} right={<Toggle on={pk} onChange={setPk} />} />
      </Panel>
      <Panel pad={26} className="col" style={{ gap: 8 }}>
        <SectionTitle title={t.acc_sessions_title} sub={t.acc_sessions_sub} />
        <div style={{ height: 8 }} />
        {sessions.map((s, i) => (
          <ListRow key={i} icon={s.icon} title={s.dev} sub={s.loc}
            right={s.cur ? <Badge dot color="hsl(150 65% 48%)" bg="hsl(150 60% 45% / .14)">{t.acc_this_device}</Badge> : <Button size="sm" variant="ghost">{t.acc_revoke}</Button>} />
        ))}
      </Panel>
    </div>
  );
}

function ServicesTab({ t }) {
  return (
    <Panel pad={28} className="col" style={{ gap: 8 }}>
      <SectionTitle title={t.acc_services_title} sub={t.acc_services_sub} />
      <div style={{ height: 10 }} />
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 6 }} className="acc-grid">
        {window.DEMO_SERVICES.map((s, i) => (
          <ListRow key={i} icon={s.icon} iconColor="#fff" title={s.name} sub={`${t.acc_connected} · ${["вчера", "3 дня назад", "сегодня", "неделю назад", "сегодня", "2 дня назад"][i]}`}
            right={<Button size="sm" variant="ghost">{t.acc_manage}</Button>}
            accent={false} />
        ))}
      </div>
    </Panel>
  );
}

function SettingsTab({ t, theme, setTheme, lang, setLang }) {
  const [notif, setNotif] = React.useState(true);
  return (
    <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 20 }} className="acc-grid">
      <Panel pad={26} className="col" style={{ gap: 20 }}>
        <SectionTitle title={t.acc_settings_title} />
        <div className="row" style={{ justifyContent: "space-between", gap: 12 }}>
          <span style={{ fontSize: 15.5, fontWeight: 600 }}>{t.acc_theme}</span>
          <Segmented size="sm" value={theme} onChange={setTheme} options={[{ value: "light", label: "", icon: "sun" }, { value: "dark", label: "", icon: "moon" }]} />
        </div>
        <div className="row" style={{ justifyContent: "space-between", gap: 12 }}>
          <span style={{ fontSize: 15.5, fontWeight: 600 }}>{t.acc_lang}</span>
          <Segmented size="sm" value={lang} onChange={setLang} options={[{ value: "ru", label: "RU" }, { value: "en", label: "EN" }]} />
        </div>
        <div style={{ height: 1, background: "var(--glass-border)" }} />
        <ListRow icon="bell" accent title={t.acc_notif} sub={t.acc_notif_sub} right={<Toggle on={notif} onChange={setNotif} />} />
      </Panel>
      <Panel pad={26} className="col" style={{ gap: 14, border: "1px solid hsl(350 70% 55% / .25)" }}>
        <SectionTitle title={t.acc_danger} />
        <ListRow icon="trash" iconColor="#ff8da3" title={t.acc_delete} sub={t.acc_delete_sub}
          right={<Button size="sm" variant="danger">{t.acc_delete}</Button>} />
      </Panel>
    </div>
  );
}

function ProfileHeader({ t, user }) {
  const [banner, setBanner] = React.useState(null);
  const [avatar, setAvatar] = React.useState(null);
  const bannerInput = React.useRef(null);
  const avatarInput = React.useRef(null);
  const pick = (ref) => ref.current && ref.current.click();
  const onFile = (e, set) => { const f = e.target.files?.[0]; if (f) set(URL.createObjectURL(f)); };
  return (
    <Panel pad={0} className="rise" style={{ overflow: "hidden", borderRadius: "var(--r-xl)" }}>
      <input ref={bannerInput} type="file" accept="image/*" hidden onChange={(e) => onFile(e, setBanner)} />
      <input ref={avatarInput} type="file" accept="image/*" hidden onChange={(e) => onFile(e, setAvatar)} />
      {/* BANNER */}
      <div style={{ position: "relative", height: 188,
        background: banner ? `center/cover url(${banner})` : "linear-gradient(120deg, var(--accent-2), var(--accent-strong) 55%, #db2777)" }}>
        {!banner && <div style={{ position: "absolute", inset: 0, background: "radial-gradient(60% 120% at 82% 0%, rgba(255,255,255,.28), transparent 55%), radial-gradient(50% 120% at 12% 120%, rgba(255,255,255,.16), transparent 55%)" }} />}
        <div style={{ position: "absolute", inset: 0, background: "linear-gradient(180deg, transparent 45%, var(--scrim))", pointerEvents: "none" }} />
        <button onClick={() => pick(bannerInput)} className="row" style={{ position: "absolute", top: 16, right: 16, gap: 8, height: 38, padding: "0 15px", borderRadius: "var(--r-pill)", background: "rgba(20,10,30,.4)", color: "#fff", border: "1px solid rgba(255,255,255,.25)", backdropFilter: "blur(14px)", fontWeight: 600, fontSize: 13.5 }}>
          <Icon name="pencil" size={16} /> {t.acc_change_banner}
        </button>
      </div>
      {/* IDENTITY */}
      <div style={{ padding: "0 30px 26px" }}>
        <div className="row acc-head" style={{ alignItems: "flex-end", gap: 20, marginTop: -52 }}>
          <div style={{ position: "relative", borderRadius: "50%", padding: 4, background: "var(--bg)", boxShadow: "0 0 0 1px var(--glass-border)", flexShrink: 0 }}>
            <Avatar name={user.name} src={avatar} size={104} />
            <button onClick={() => pick(avatarInput)} title={t.acc_change_avatar} style={{ position: "absolute", right: 2, bottom: 2, width: 34, height: 34, borderRadius: "50%", display: "grid", placeItems: "center", background: "linear-gradient(135deg, var(--accent-2), var(--accent-strong))", color: "#fff", border: "3px solid var(--bg)", boxShadow: "0 4px 12px -4px var(--accent-strong)" }}>
              <Icon name="pencil" size={15} />
            </button>
          </div>
          <div className="col" style={{ gap: 5, flex: 1, minWidth: 0, paddingBottom: 6 }}>
            <div className="row" style={{ gap: 13, flexWrap: "wrap" }}>
              <h1 style={{ fontFamily: "var(--serif)", fontWeight: 400, fontSize: 32, lineHeight: 1, whiteSpace: "nowrap" }}>{user.name}</h1>
              <Badge dot color="var(--accent)" bg="var(--accent-soft)">@{user.username}</Badge>
            </div>
            <span style={{ fontSize: 14.5, color: "var(--text-3)" }}>{user.email} · {t.acc_member_since} {user.since}</span>
          </div>
          <div style={{ paddingBottom: 6 }}><Button variant="glass" icon="pencil">{t.acc_edit}</Button></div>
        </div>
      </div>
    </Panel>
  );
}

function ScreenAccount() {
  const { t, lang, theme, setTheme, setLang, go, user } = useAppAcc();
  const [tab, setTab] = React.useState("profile");
  const tabs = [
    { value: "profile", label: t.acc_nav_profile, icon: "user" },
    { value: "security", label: t.acc_nav_security, icon: "shield" },
    { value: "services", label: t.acc_nav_services, icon: "layers" },
    { value: "settings", label: t.acc_nav_settings, icon: "settings" },
  ];
  return (
    <div className="screen-enter">
      <NavBar t={t} theme={theme} setTheme={setTheme} lang={lang} setLang={setLang} go={go} user={user} />
      <div style={{ maxWidth: 1080, margin: "0 auto", padding: "104px 24px 60px" }}>
        <ProfileHeader t={t} user={user} />

        {/* TABS */}
        <div className="row" style={{ margin: "26px 0 22px", justifyContent: "center" }}>
          <Segmented value={tab} onChange={setTab} options={tabs} />
        </div>

        <div key={tab} className="screen-enter">
          {tab === "profile" && <ProfileTab t={t} user={user} />}
          {tab === "security" && <SecurityTab t={t} user={user} />}
          {tab === "services" && <ServicesTab t={t} />}
          {tab === "settings" && <SettingsTab t={t} theme={theme} setTheme={setTheme} lang={lang} setLang={setLang} />}
        </div>
      </div>
    </div>
  );
}

window.ScreenAccount = ScreenAccount;
