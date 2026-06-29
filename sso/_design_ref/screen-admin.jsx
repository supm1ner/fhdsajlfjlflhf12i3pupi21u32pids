// screen-admin.jsx — Admin console: dashboard, users, user card
function useAppAdm() { return React.useContext(window.AppCtx); }

const ADMIN_USERS = [
  { name: "Alex Renn", username: "alex", email: "alex@cotton-id.io", status: "active", role: "owner", services: 12, joined: "12 фев 2024", logins: 842, last: "2 мин назад", loc: "Алматы, KZ", about: "Builder of small internet things." },
  { name: "Mira Voss", username: "mira", email: "mira.voss@mail.io", status: "active", role: "admin", services: 8, joined: "03 мар 2024", logins: 511, last: "1 ч назад", loc: "Берлин, DE", about: "Design systems & coffee." },
  { name: "Дамир Алиев", username: "damir", email: "damir@studio.kz", status: "active", role: "user", services: 5, joined: "21 апр 2024", logins: 207, last: "вчера", loc: "Астана, KZ", about: "" },
  { name: "Sora Tan", username: "sora", email: "sora@inbox.jp", status: "invited", role: "user", services: 1, joined: "08 май 2024", logins: 3, last: "—", loc: "Токио, JP", about: "" },
  { name: "Lena Park", username: "lpark", email: "lena.park@cloud.io", status: "active", role: "user", services: 7, joined: "30 май 2024", logins: 388, last: "12 мин назад", loc: "Сеул, KR", about: "Frontend & motion." },
  { name: "Tom Wilder", username: "twild", email: "tom@wilder.dev", status: "suspended", role: "user", services: 2, joined: "14 июн 2024", logins: 94, last: "5 дней назад", loc: "Остин, US", about: "" },
  { name: "Aiya Nur", username: "aiya", email: "aiya@cotton-id.io", status: "active", role: "admin", services: 9, joined: "02 июл 2024", logins: 623, last: "3 ч назад", loc: "Алматы, KZ", about: "Support lead." },
  { name: "Ravi Mehta", username: "ravi", email: "ravi.m@mail.in", status: "active", role: "user", services: 4, joined: "19 авг 2024", logins: 142, last: "вчера", loc: "Мумбаи, IN", about: "" },
];

function StatusBadge({ status, t }) {
  const map = { active: t.adm_st_active, suspended: t.adm_st_suspended, invited: t.adm_st_invited };
  const c = window.STATUS_COLOR[status];
  return <Badge dot color={c[0]} bg={c[1]}>{map[status]}</Badge>;
}
function RoleBadge({ role, t }) {
  const map = { user: t.adm_role_user, admin: t.adm_role_admin, owner: t.adm_role_owner };
  const isPriv = role !== "user";
  return <Badge color={isPriv ? "var(--accent)" : "var(--text-2)"} bg={isPriv ? "var(--accent-soft)" : "var(--glass-2)"}>{map[role]}</Badge>;
}

/* ----------------- SHELL ----------------- */
function AdminShell({ active, onNav, t, theme, setTheme, lang, setLang, go, search, setSearch, children, title }) {
  const nav = [
    { id: "dashboard", label: t.adm_nav_overview, icon: "grid" },
    { id: "users", label: t.adm_nav_users, icon: "users" },
    { id: "services", label: t.adm_nav_services, icon: "layers" },
    { id: "logs", label: t.adm_nav_logs, icon: "activity" },
    { id: "settings", label: t.adm_nav_settings, icon: "settings" },
  ];
  return (
    <div className="screen-enter admin-shell" style={{ display: "grid", gridTemplateColumns: "264px 1fr", minHeight: "100vh" }}>
      {/* SIDEBAR */}
      <aside style={{ padding: 18, position: "sticky", top: 0, height: "100vh" }} className="admin-side">
        <div className="glass col" style={{ height: "100%", borderRadius: "var(--r-lg)", padding: 18, gap: 6 }}>
          <div className="row" style={{ gap: 10, padding: "8px 10px 18px" }}>
            <Logo size={26} label={false} />
            <div className="col" style={{ gap: 0, lineHeight: 1 }}>
              <span style={{ fontFamily: "var(--serif)", fontSize: 21, color: "var(--text)", whiteSpace: "nowrap" }}>cotton-id</span>
              <span style={{ fontSize: 11.5, color: "var(--accent)", fontWeight: 700, letterSpacing: ".14em", textTransform: "uppercase" }}>{t.adm_brand}</span>
            </div>
          </div>
          {nav.map((n) => {
            const on = active === n.id || (active === "user" && n.id === "users");
            return (
              <button key={n.id} onClick={() => onNav(n.id)} className="row" style={{
                gap: 12, padding: "12px 14px", borderRadius: "var(--r-sm)", fontSize: 15, fontWeight: 600,
                color: on ? "var(--accent-ink)" : "var(--text-2)",
                background: on ? "linear-gradient(135deg, var(--accent-2), var(--accent-strong))" : "transparent",
                boxShadow: on ? "0 8px 20px -10px var(--accent-strong)" : "none",
                transition: "all .35s var(--ease)",
              }}
                onMouseEnter={(e) => { if (!on) e.currentTarget.style.background = "var(--glass-2)"; }}
                onMouseLeave={(e) => { if (!on) e.currentTarget.style.background = "transparent"; }}>
                <Icon name={n.icon} size={20} /> {n.label}
              </button>
            );
          })}
          <div style={{ flex: 1 }} />
          <button onClick={() => go("account")} className="row" style={{ gap: 12, padding: "12px 14px", borderRadius: "var(--r-sm)", color: "var(--text-2)", fontWeight: 600, fontSize: 15 }}
            onMouseEnter={(e) => e.currentTarget.style.background = "var(--glass-2)"} onMouseLeave={(e) => e.currentTarget.style.background = "transparent"}>
            <Icon name="logout" size={20} /> {lang === "ru" ? "Выйти в приложение" : "Back to app"}
          </button>
        </div>
      </aside>

      {/* MAIN */}
      <main style={{ padding: "18px 26px 40px", minWidth: 0 }}>
        {/* topbar */}
        <div className="row" style={{ gap: 16, marginBottom: 24, justifyContent: "space-between" }}>
          <div className="glass row" style={{ gap: 11, flex: 1, maxWidth: 420, padding: "0 18px", height: 48, borderRadius: "var(--r-pill)" }}>
            <Icon name="search" size={19} style={{ color: "var(--text-3)" }} />
            <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder={t.adm_search}
              style={{ flex: 1, border: "none", background: "transparent", outline: "none", color: "var(--text)", fontSize: 15 }} />
          </div>
          <div className="row" style={{ gap: 9 }}>
            <IconBtn name="bell" />
            <LangSwitch lang={lang} setLang={setLang} />
            <ThemeSwitch theme={theme} setTheme={setTheme} />
            <div className="glass row" style={{ gap: 10, padding: "5px 14px 5px 6px", borderRadius: "var(--r-pill)" }}>
              <Avatar name="Alex Renn" size={34} />
              <div className="col" style={{ gap: 0, lineHeight: 1.15 }}>
                <span style={{ fontSize: 13.5, fontWeight: 700, whiteSpace: "nowrap" }}>Alex Renn</span>
                <span style={{ fontSize: 11.5, color: "var(--accent)" }}>{t.adm_role_owner}</span>
              </div>
            </div>
          </div>
        </div>
        {children}
      </main>
    </div>
  );
}

/* ----------------- DASHBOARD ----------------- */
function StatCard({ icon, label, value, delta, up, delay }) {
  return (
    <Panel pad={22} className="rise col" style={{ gap: 14, animationDelay: `${delay}s` }}>
      <div className="row" style={{ justifyContent: "space-between" }}>
        <div style={{ width: 44, height: 44, borderRadius: 13, display: "grid", placeItems: "center", background: "var(--accent-soft)", color: "var(--accent)", border: "1px solid var(--accent-line)" }}><Icon name={icon} size={21} /></div>
        {delta && <Badge color={up ? "hsl(150 65% 48%)" : "#ff8da3"} bg={up ? "hsl(150 60% 45% / .14)" : "hsl(350 75% 60% / .14)"}>{up ? "↑" : "↓"} {delta}</Badge>}
      </div>
      <div className="col" style={{ gap: 2 }}>
        <span style={{ fontFamily: "var(--serif)", fontSize: 40, lineHeight: 1, color: "var(--text)" }}>{value}</span>
        <span style={{ fontSize: 14, color: "var(--text-3)" }}>{label}</span>
      </div>
    </Panel>
  );
}

function SignupChart({ t }) {
  const bars = React.useMemo(() => Array.from({ length: 30 }, (_, i) => 30 + Math.round(40 * Math.abs(Math.sin(i * 0.7)) + (i / 30) * 30 + (i % 5) * 4)), []);
  const max = Math.max(...bars);
  return (
    <Panel pad={26} className="col" style={{ gap: 20 }}>
      <div className="row" style={{ justifyContent: "space-between" }}>
        <SectionTitleAdm title={t.adm_chart_title} sub={t.adm_chart_sub} />
        <Badge color="var(--accent)" bg="var(--accent-soft)" dot>+18.4%</Badge>
      </div>
      <div className="row" style={{ gap: 5, alignItems: "flex-end", height: 160 }}>
        {bars.map((b, i) => (
          <div key={i} title={`${b}`} className="chart-bar" style={{ flex: 1, height: `${(b / max) * 100}%`, borderRadius: "5px 5px 3px 3px",
            background: i >= 25 ? "linear-gradient(180deg, var(--accent-2), var(--accent-strong))" : "var(--glass-3)",
            boxShadow: i >= 25 ? "0 6px 14px -6px var(--accent-strong)" : "none", transition: "all .3s",
            animationDelay: `${i * 0.018}s` }} />
        ))}
      </div>
    </Panel>
  );
}

function SectionTitleAdm({ title, sub }) {
  return (
    <div className="col" style={{ gap: 3 }}>
      <h3 style={{ fontSize: 19, fontWeight: 600 }}>{title}</h3>
      {sub && <span style={{ fontSize: 13.5, color: "var(--text-3)" }}>{sub}</span>}
    </div>
  );
}

function DashboardView({ t, onUser }) {
  const activity = [
    { who: "Mira Voss", what: langAct(t, "signed_in", "Mira Voss"), icon: "key", time: "2 мин" },
    { who: "Sora Tan", what: langAct(t, "invited", "Sora Tan"), icon: "mail", time: "18 мин" },
    { who: "Tom Wilder", what: langAct(t, "suspended", "Tom Wilder"), icon: "lock", time: "1 ч" },
    { who: "Lena Park", what: langAct(t, "connected", "Lena Park"), icon: "link", time: "2 ч" },
    { who: "Ravi Mehta", what: langAct(t, "signed_in", "Ravi Mehta"), icon: "key", time: "3 ч" },
  ];
  return (
    <div className="col" style={{ gap: 22 }}>
      <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-end" }}>
        <h1 style={{ fontFamily: "var(--serif)", fontWeight: 400, fontSize: 34, lineHeight: 1 }}>{t.adm_overview}</h1>
        <span style={{ fontSize: 13.5, color: "var(--text-3)" }}>{t.just_demo}</span>
      </div>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 18 }} className="adm-stats">
        <StatCard icon="users" label={t.adm_stat_users} value="12,847" delta="6.2%" up delay={0} />
        <StatCard icon="bolt" label={t.adm_stat_active} value="3,210" delta="2.1%" up delay={0.06} />
        <StatCard icon="sparkle" label={t.adm_stat_new} value="482" delta="18%" up delay={0.12} />
        <StatCard icon="layers" label={t.adm_stat_services} value="48" delta="3" up delay={0.18} />
      </div>
      <div style={{ display: "grid", gridTemplateColumns: "1.5fr 1fr", gap: 18 }} className="adm-grid">
        <SignupChart t={t} />
        <Panel pad={24} className="col" style={{ gap: 6 }}>
          <div className="row" style={{ justifyContent: "space-between", marginBottom: 6 }}>
            <SectionTitleAdm title={t.adm_recent_title} />
            <button onClick={() => onUser(null)} style={{ fontSize: 13.5, color: "var(--accent)", fontWeight: 700 }}>{t.adm_view_all} →</button>
          </div>
          {ADMIN_USERS.slice(0, 5).map((u, i) => (
            <div key={i} onClick={() => onUser(u)} className="row" style={{ gap: 12, padding: "10px 10px", borderRadius: "var(--r-sm)", cursor: "pointer", transition: "background .3s" }}
              onMouseEnter={(e) => e.currentTarget.style.background = "var(--glass-2)"} onMouseLeave={(e) => e.currentTarget.style.background = "transparent"}>
              <Avatar name={u.name} size={38} />
              <div className="col" style={{ gap: 1, flex: 1, minWidth: 0 }}>
                <span style={{ fontSize: 14.5, fontWeight: 600 }}>{u.name}</span>
                <span style={{ fontSize: 12.5, color: "var(--text-3)" }}>@{u.username}</span>
              </div>
              <StatusBadge status={u.status} t={t} />
            </div>
          ))}
        </Panel>
      </div>
      <Panel pad={24} className="col" style={{ gap: 6 }}>
        <SectionTitleAdm title={t.adm_activity_title} />
        <div style={{ height: 8 }} />
        {activity.map((a, i) => (
          <div key={i} className="row" style={{ gap: 13, padding: "10px 6px" }}>
            <div style={{ width: 36, height: 36, borderRadius: 11, display: "grid", placeItems: "center", background: "var(--glass-2)", color: "var(--accent)", border: "1px solid var(--glass-border)" }}><Icon name={a.icon} size={17} /></div>
            <span style={{ flex: 1, fontSize: 14.5, color: "var(--text-2)" }}>{a.what}</span>
            <span style={{ fontSize: 13, color: "var(--text-3)" }}>{a.time}</span>
          </div>
        ))}
      </Panel>
    </div>
  );
}

function langAct(t, kind, name) {
  const ru = { signed_in: `${name} вошёл в аккаунт`, invited: `${name} приглашён(а)`, suspended: `${name} заблокирован(а)`, connected: `${name} подключил(а) сервис` };
  const en = { signed_in: `${name} signed in`, invited: `${name} was invited`, suspended: `${name} was suspended`, connected: `${name} connected a service` };
  return (t._lang === "en" ? en : ru)[kind];
}

/* ----------------- USERS LIST ----------------- */
function UsersView({ t, onUser, search }) {
  const [filter, setFilter] = React.useState("all");
  const filtered = ADMIN_USERS.filter((u) =>
    (filter === "all" || u.status === filter) &&
    (!search || (u.name + u.username + u.email).toLowerCase().includes(search.toLowerCase()))
  );
  return (
    <div className="col" style={{ gap: 22 }}>
      <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-end", flexWrap: "wrap", gap: 14 }}>
        <div className="col" style={{ gap: 4 }}>
          <h1 style={{ fontFamily: "var(--serif)", fontWeight: 400, fontSize: 34, lineHeight: 1 }}>{t.adm_users_title}</h1>
          <p style={{ fontSize: 14.5, color: "var(--text-3)" }}>{t.adm_users_sub}</p>
        </div>
        <div className="row" style={{ gap: 12 }}>
          <Segmented size="sm" value={filter} onChange={setFilter} options={[
            { value: "all", label: t.adm_filter_all }, { value: "active", label: t.adm_filter_active }, { value: "suspended", label: t.adm_filter_suspended },
          ]} />
          <Button size="sm" icon="plus">{t.nav_signup}</Button>
        </div>
      </div>
      <Panel pad={0} style={{ overflow: "hidden" }}>
        {/* head */}
        <div className="row adm-thead" style={{ gap: 14, padding: "16px 24px", fontSize: 12.5, fontWeight: 700, color: "var(--text-3)", textTransform: "uppercase", letterSpacing: ".05em", borderBottom: "1px solid var(--glass-border)" }}>
          <span style={{ flex: 2.4 }}>{t.adm_th_user}</span>
          <span style={{ flex: 1 }}>{t.adm_th_status}</span>
          <span style={{ flex: 1 }}>{t.adm_th_role}</span>
          <span style={{ flex: .8, textAlign: "center" }}>{t.adm_th_services}</span>
          <span style={{ flex: 1.1 }}>{t.adm_th_joined}</span>
          <span style={{ width: 30 }} />
        </div>
        {filtered.map((u, i) => (
          <div key={i} onClick={() => onUser(u)} className="row adm-row" style={{ gap: 14, padding: "13px 24px", cursor: "pointer", transition: "background .25s", borderBottom: i < filtered.length - 1 ? "1px solid var(--glass-border)" : "none" }}
            onMouseEnter={(e) => e.currentTarget.style.background = "var(--glass-2)"} onMouseLeave={(e) => e.currentTarget.style.background = "transparent"}>
            <div className="row" style={{ flex: 2.4, gap: 13, minWidth: 0 }}>
              <Avatar name={u.name} size={42} />
              <div className="col" style={{ gap: 1, minWidth: 0 }}>
                <span style={{ fontSize: 15, fontWeight: 600 }}>{u.name}</span>
                <span style={{ fontSize: 13, color: "var(--text-3)", overflow: "hidden", textOverflow: "ellipsis" }}>{u.email}</span>
              </div>
            </div>
            <div style={{ flex: 1 }}><StatusBadge status={u.status} t={t} /></div>
            <div style={{ flex: 1 }}><RoleBadge role={u.role} t={t} /></div>
            <span style={{ flex: .8, textAlign: "center", fontSize: 15, fontWeight: 600, color: "var(--text-2)" }}>{u.services}</span>
            <span style={{ flex: 1.1, fontSize: 13.5, color: "var(--text-3)" }}>{u.joined}</span>
            <Icon name="chevron" size={18} style={{ width: 30, color: "var(--text-3)" }} />
          </div>
        ))}
      </Panel>
    </div>
  );
}

/* ----------------- USER CARD ----------------- */
function UserCard({ t, user, onBack }) {
  const u = user;
  const sessions = [
    { dev: "MacBook Pro · Safari", loc: u.loc, icon: "monitor", cur: true },
    { dev: "iPhone · cotton-id", loc: u.loc, icon: "phone", cur: false },
  ];
  const en = t._lang === "en";
  const acts = [
    { what: u.name + (en ? " signed in" : " вошёл в аккаунт"), time: u.last, icon: "key" },
    { what: (en ? "Connected " : "Подключён ") + "Vault", time: en ? "2 days" : "2 дня", icon: "link" },
    { what: en ? "Enabled 2FA" : "Включена 2FA", time: en ? "1 week" : "1 нед", icon: "shield" },
  ];
  return (
    <div className="col" style={{ gap: 22 }}>
      <button onClick={onBack} className="row" style={{ gap: 8, fontSize: 14.5, fontWeight: 600, color: "var(--text-2)", width: "fit-content" }}>
        <Icon name="back" size={18} /> {t.adm_back}
      </button>
      <div style={{ display: "grid", gridTemplateColumns: "1.6fr 1fr", gap: 18 }} className="adm-grid">
        <div className="col" style={{ gap: 18 }}>
          {/* header */}
          <Panel pad={0} style={{ overflow: "hidden" }}>
            <div style={{ height: 130, background: "linear-gradient(120deg, var(--accent-2), var(--accent-strong) 60%, #db2777)", position: "relative" }}>
              <div style={{ position: "absolute", inset: 0, background: "radial-gradient(120% 120% at 85% 0%, rgba(255,255,255,.3), transparent 50%)" }} />
            </div>
            <div style={{ padding: "0 26px 24px" }}>
              <div className="row" style={{ alignItems: "flex-end", gap: 16, marginTop: -42 }}>
                <div style={{ borderRadius: "50%", padding: 4, background: "var(--bg)" }}><Avatar name={u.name} size={84} /></div>
                <div className="col" style={{ gap: 4, flex: 1, paddingBottom: 4 }}>
                  <span style={{ fontSize: 24, fontWeight: 700 }}>{u.name}</span>
                  <span style={{ fontSize: 14, color: "var(--text-3)" }}>@{u.username} · {u.email}</span>
                </div>
                <div className="row" style={{ gap: 8, paddingBottom: 4 }}><StatusBadge status={u.status} t={t} /><RoleBadge role={u.role} t={t} /></div>
              </div>
              {u.about && <p style={{ marginTop: 16, fontSize: 14.5, color: "var(--text-2)", lineHeight: 1.5 }}>{u.about}</p>}
              <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(130px, 1fr))", gap: 18, marginTop: 18 }}>
                {[[u.logins, t.adm_logins], [u.services, t.adm_card_services], [u.joined, t.adm_th_joined], [u.loc, t.acc_loc]].map((s, i) => (
                  <div key={i} className="col" style={{ gap: 1, minWidth: 0 }}>
                    <span style={{ fontFamily: "var(--serif)", fontSize: 22, color: "var(--text)", lineHeight: 1.1, whiteSpace: "nowrap" }}>{s[0]}</span>
                    <span style={{ fontSize: 12.5, color: "var(--text-3)", whiteSpace: "nowrap" }}>{s[1]}</span>
                  </div>
                ))}
              </div>
            </div>
          </Panel>
          {/* sessions */}
          <Panel pad={24} className="col" style={{ gap: 6 }}>
            <SectionTitleAdm title={t.adm_card_sessions} />
            <div style={{ height: 6 }} />
            {sessions.map((s, i) => (
              <ListRow key={i} icon={s.icon} title={s.dev} sub={s.loc}
                right={s.cur ? <Badge dot color="hsl(150 65% 48%)" bg="hsl(150 60% 45% / .14)">{t.acc_this_device}</Badge> : <Button size="sm" variant="ghost">{t.acc_revoke}</Button>} />
            ))}
          </Panel>
          {/* activity */}
          <Panel pad={24} className="col" style={{ gap: 6 }}>
            <SectionTitleAdm title={t.adm_card_activity} />
            <div style={{ height: 6 }} />
            {acts.map((a, i) => (
              <div key={i} className="row" style={{ gap: 13, padding: "10px 6px" }}>
                <div style={{ width: 34, height: 34, borderRadius: 10, display: "grid", placeItems: "center", background: "var(--glass-2)", color: "var(--accent)", border: "1px solid var(--glass-border)" }}><Icon name={a.icon} size={16} /></div>
                <span style={{ flex: 1, fontSize: 14.5, color: "var(--text-2)" }}>{a.what}</span>
                <span style={{ fontSize: 13, color: "var(--text-3)" }}>{a.time}</span>
              </div>
            ))}
          </Panel>
        </div>
        {/* side */}
        <div className="col" style={{ gap: 18 }}>
          <Panel pad={22} className="col" style={{ gap: 11 }}>
            <SectionTitleAdm title={t.adm_card_actions} />
            <div style={{ height: 4 }} />
            <Button variant="glass" full icon="mail" style={{ justifyContent: "flex-start" }}>{t.adm_act_message}</Button>
            <Button variant="glass" full icon="key" style={{ justifyContent: "flex-start" }}>{t.adm_act_reset}</Button>
            <Button variant="glass" full icon="lock" style={{ justifyContent: "flex-start" }}>{t.adm_act_suspend}</Button>
            <Button variant="danger" full icon="trash" style={{ justifyContent: "flex-start" }}>{t.adm_act_delete}</Button>
          </Panel>
          <Panel pad={22} className="col" style={{ gap: 6 }}>
            <SectionTitleAdm title={t.adm_card_services} />
            <div style={{ height: 6 }} />
            {window.DEMO_SERVICES.slice(0, u.services > 5 ? 5 : u.services).map((s, i) => (
              <div key={i} className="row" style={{ gap: 12, padding: "8px 4px" }}>
                <div style={{ width: 34, height: 34, borderRadius: 10, display: "grid", placeItems: "center", color: "#fff", background: `linear-gradient(140deg, ${s.c[0]}, ${s.c[1]})` }}><Icon name={s.icon} size={16} /></div>
                <span style={{ flex: 1, fontSize: 14.5, fontWeight: 600 }}>{s.name}</span>
                <Icon name="check" size={16} style={{ color: "hsl(150 60% 50%)" }} />
              </div>
            ))}
          </Panel>
        </div>
      </div>
    </div>
  );
}

/* ----------------- ROOT ----------------- */
function ScreenAdmin() {
  const { t, lang, theme, setTheme, setLang, go } = useAppAdm();
  const [view, setView] = React.useState("dashboard");
  const [selected, setSelected] = React.useState(null);
  const [search, setSearch] = React.useState("");
  const openUser = (u) => { if (u) { setSelected(u); setView("user"); } else { setView("users"); } };
  return (
    <AdminShell active={view} onNav={(id) => { setView(id === "services" || id === "logs" || id === "settings" ? "dashboard" : id); }}
      t={t} theme={theme} setTheme={setTheme} lang={lang} setLang={setLang} go={go} search={search} setSearch={setSearch}>
      <div key={view + (selected?.username || "")} className="screen-enter">
        {view === "dashboard" && <DashboardView t={t} onUser={openUser} />}
        {view === "users" && <UsersView t={t} onUser={openUser} search={search} />}
        {view === "user" && selected && <UserCard t={t} user={selected} onBack={() => setView("users")} />}
      </div>
    </AdminShell>
  );
}

window.ScreenAdmin = ScreenAdmin;
window.ADMIN_USERS = ADMIN_USERS;
