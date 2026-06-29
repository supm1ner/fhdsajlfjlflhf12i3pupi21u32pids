// ui.jsx — shared liquid-glass UI kit for cotton-id
const { useState, useEffect, useRef } = React;

/* ----------------------------- ICONS ----------------------------- */
const ICONS = {
  arrow: "M5 12h14M13 6l6 6-6 6",
  check: "M4 12l5 5L20 6",
  shield: "M12 3l7 3v5c0 4.5-3 7.5-7 9-4-1.5-7-4.5-7-9V6l7-3z",
  key: "M14 7a4 4 0 1 0-3.5 6.9L7 17.5V20h2.5l.5-2h2l1-1v-2l1.5-1.5A4 4 0 0 0 14 7z",
  user: "M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM5 20c0-3.3 3.1-5 7-5s7 1.7 7 5",
  layers: "M12 3l9 5-9 5-9-5 9-5zM3 13l9 5 9-5M3 17l9 5 9-5",
  lock: "M6 11V8a6 6 0 0 1 12 0v3M5 11h14v9H5z",
  bell: "M6 9a6 6 0 0 1 12 0c0 5 2 6 2 6H4s2-1 2-6M10 21a2 2 0 0 0 4 0",
  trash: "M4 7h16M9 7V5a2 2 0 0 1 2-2h2a2 2 0 0 1 2 2v2M6 7l1 13h10l1-13",
  search: "M11 19a8 8 0 1 0 0-16 8 8 0 0 0 0 16zM21 21l-4.3-4.3",
  sun: "M12 4V2M12 22v-2M5 5L4 4M20 20l-1-1M4 12H2M22 12h-2M5 19l-1 1M20 4l-1 1M12 17a5 5 0 1 0 0-10 5 5 0 0 0 0 10z",
  moon: "M21 13A8.5 8.5 0 1 1 11 3a6.5 6.5 0 0 0 10 10z",
  globe: "M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM3 12h18M12 3c2.5 2.7 2.5 15.3 0 18M12 3c-2.5 2.7-2.5 15.3 0 18",
  x: "M6 6l12 12M18 6L6 18",
  chevron: "M9 6l6 6-6 6",
  chevdown: "M6 9l6 6 6-6",
  logout: "M15 4h4v16h-4M11 8l-4 4 4 4M7 12h10",
  grid: "M4 4h7v7H4zM13 4h7v7h-7zM4 13h7v7H4zM13 13h7v7h-7z",
  users: "M16 12a4 4 0 1 0-4-4M9 13a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM2 21c0-3.3 3.1-5 7-5s7 1.7 7 5M16 16c2.7.3 5 1.7 5 4",
  activity: "M3 12h4l3 8 4-16 3 8h4",
  more: "M5 12h.01M12 12h.01M19 12h.01",
  mail: "M3 6h18v12H3zM3 7l9 6 9-6",
  monitor: "M3 4h18v12H3zM8 20h8M12 16v4",
  phone: "M8 3h8v18H8zM11 18h2",
  plus: "M12 5v14M5 12h14",
  sparkle: "M12 3l1.8 5.2L19 10l-5.2 1.8L12 17l-1.8-5.2L5 10l5.2-1.8L12 3z",
  finger: "M12 11v3a4 4 0 0 1-4 4M8 11a4 4 0 0 1 8 0v2M5 11a7 7 0 0 1 14 0v1M12 18v2",
  eye: "M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7zM12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z",
  settings: "M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM19 12a7 7 0 0 0-.1-1.3l2-1.5-2-3.4-2.3.9a7 7 0 0 0-2.2-1.3L14 2h-4l-.4 2.4a7 7 0 0 0-2.2 1.3L5 4.8 3 8.2l2 1.5a7 7 0 0 0 0 2.6l-2 1.5 2 3.4 2.3-.9a7 7 0 0 0 2.2 1.3L10 22h4l.4-2.4a7 7 0 0 0 2.2-1.3l2.3.9 2-3.4-2-1.5A7 7 0 0 0 19 12z",
  bolt: "M13 2L4 14h7l-1 8 9-12h-7l1-8z",
  star: "M12 3l2.6 6.3L21 10l-5 4.3 1.5 6.7L12 17.5 6.5 21 8 14.3 3 10l6.4-.7L12 3z",
  link: "M9 15l6-6M10 6l1-1a4 4 0 0 1 6 6l-1 1M14 18l-1 1a4 4 0 0 1-6-6l1-1",
  doc: "M6 2h8l4 4v16H6zM14 2v4h4",
  clock: "M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM12 7v5l3 2",
  pencil: "M4 20l4-1L19 8l-3-3L5 16l-1 4zM14 6l3 3",
  back: "M19 12H5M11 6l-6 6 6 6",
};

function Icon({ name, size = 20, stroke = 1.7, fill, style, className }) {
  const d = ICONS[name];
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none"
      stroke={fill ? "none" : "currentColor"} strokeWidth={stroke}
      strokeLinecap="round" strokeLinejoin="round" style={style} className={className}>
      <path d={d} fill={fill || "none"} />
    </svg>
  );
}

/* ----------------------------- LOGO ----------------------------- */
function Logo({ size = 30, onClick, mark = true, label = true }) {
  return (
    <div className="row" style={{ gap: 11, cursor: onClick ? "pointer" : "default" }} onClick={onClick}>
      {mark && (
        <div style={{
          width: size, height: size, borderRadius: size * 0.32, position: "relative",
          background: "linear-gradient(140deg, var(--accent-2), var(--accent-strong))",
          boxShadow: "0 6px 18px -6px var(--accent-strong), inset 0 1px 0 rgba(255,255,255,.5)",
          display: "grid", placeItems: "center", flexShrink: 0,
        }}>
          <div style={{
            width: size * 0.42, height: size * 0.42, borderRadius: "50%",
            background: "radial-gradient(circle at 35% 30%, #fff, rgba(255,255,255,.4))",
            boxShadow: "0 0 12px rgba(255,255,255,.6)",
          }} />
        </div>
      )}
      {label && (
        <span style={{ fontFamily: "var(--serif)", fontSize: size * 0.82, letterSpacing: ".01em", color: "var(--text)", lineHeight: 1, whiteSpace: "nowrap" }}>
          cotton<span style={{ color: "var(--accent)" }}>-</span>id
        </span>
      )}
    </div>
  );
}

/* ----------------------------- BUTTON ----------------------------- */
function Button({ children, variant = "primary", size = "md", icon, iconRight, full, onClick, type, style }) {
  const sizes = {
    sm: { padding: "8px 16px", fontSize: 14, height: 38, radius: "var(--r-pill)" },
    md: { padding: "0 22px", fontSize: 15.5, height: 48, radius: "var(--r-pill)" },
    lg: { padding: "0 30px", fontSize: 17, height: 58, radius: "var(--r-pill)" },
  }[size];
  const base = {
    display: "inline-flex", alignItems: "center", justifyContent: "center", gap: 9,
    height: sizes.height, padding: sizes.padding, fontSize: sizes.fontSize, fontWeight: 600,
    borderRadius: sizes.radius, width: full ? "100%" : undefined, whiteSpace: "nowrap",
    transition: "transform .35s var(--ease), box-shadow .35s var(--ease), background .3s, opacity .3s",
    letterSpacing: ".005em", position: "relative", ...style,
  };
  const variants = {
    primary: {
      background: "linear-gradient(135deg, var(--accent-2), var(--accent-strong))",
      color: "var(--accent-ink)",
      boxShadow: "0 10px 28px -10px var(--accent-strong), inset 0 1px 0 rgba(255,255,255,.45)",
    },
    glass: {
      background: "var(--glass-2)", color: "var(--text)",
      border: "1px solid var(--glass-border-2)", backdropFilter: "blur(20px)",
      boxShadow: "inset 0 1px 0 var(--glass-hi)",
    },
    ghost: { background: "transparent", color: "var(--text-2)" },
    danger: {
      background: "hsl(350 80% 55% / .14)", color: "#ff8da3",
      border: "1px solid hsl(350 80% 60% / .3)",
    },
  };
  return (
    <button type={type} onClick={onClick}
      style={{ ...base, ...variants[variant] }}
      onMouseEnter={(e) => { e.currentTarget.style.transform = "translateY(-2px)"; }}
      onMouseLeave={(e) => { e.currentTarget.style.transform = "none"; }}
      onMouseDown={(e) => { e.currentTarget.style.transform = "translateY(0) scale(.97)"; }}
      onMouseUp={(e) => { e.currentTarget.style.transform = "translateY(-2px)"; }}>
      {icon && <Icon name={icon} size={size === "lg" ? 20 : 18} />}
      {children}
      {iconRight && <Icon name={iconRight} size={size === "lg" ? 20 : 18} />}
    </button>
  );
}

function IconBtn({ name, onClick, active, size = 42, title, style }) {
  return (
    <button title={title} onClick={onClick} style={{
      width: size, height: size, borderRadius: "50%", display: "grid", placeItems: "center",
      background: active ? "var(--accent-soft)" : "var(--glass-2)",
      border: `1px solid ${active ? "var(--accent-line)" : "var(--glass-border)"}`,
      color: active ? "var(--accent)" : "var(--text-2)",
      backdropFilter: "blur(18px)", boxShadow: "inset 0 1px 0 var(--glass-hi)",
      transition: "all .35s var(--ease)", ...style,
    }}
      onMouseEnter={(e) => { e.currentTarget.style.color = "var(--text)"; e.currentTarget.style.transform = "translateY(-2px)"; }}
      onMouseLeave={(e) => { e.currentTarget.style.color = active ? "var(--accent)" : "var(--text-2)"; e.currentTarget.style.transform = "none"; }}>
      <Icon name={name} size={size * 0.45} />
    </button>
  );
}

/* ----------------------------- INPUT ----------------------------- */
function Field({ label, type = "text", value, onChange, icon, hint, right, autoFocus }) {
  const [focus, setFocus] = useState(false);
  const has = value && value.length > 0;
  return (
    <label className="col" style={{ gap: 7 }}>
      <span style={{ fontSize: 13.5, fontWeight: 600, color: "var(--text-2)", letterSpacing: ".01em", paddingLeft: 2 }}>{label}</span>
      <div className="row" style={{
        gap: 11, padding: "0 16px", height: 54, borderRadius: "var(--r-sm)",
        background: "var(--field)",
        border: `1.5px solid ${focus ? "var(--accent-line)" : "var(--glass-border)"}`,
        boxShadow: focus ? "0 0 0 4px var(--accent-soft), inset 0 1px 0 var(--glass-hi)" : "inset 0 1px 0 var(--glass-hi)",
        transition: "all .3s var(--ease)", backdropFilter: "blur(16px)",
      }}>
        {icon && <Icon name={icon} size={19} style={{ color: focus ? "var(--accent)" : "var(--text-3)", transition: "color .3s", flexShrink: 0 }} />}
        <input type={type} value={value} autoFocus={autoFocus}
          onChange={(e) => onChange && onChange(e.target.value)}
          onFocus={() => setFocus(true)} onBlur={() => setFocus(false)}
          style={{ flex: 1, border: "none", outline: "none", background: "transparent", color: "var(--text)", fontSize: 16, fontWeight: 500, minWidth: 0 }} />
        {right}
      </div>
      {hint && <span style={{ fontSize: 12.5, color: "var(--text-3)", paddingLeft: 2 }}>{hint}</span>}
    </label>
  );
}

/* ----------------------------- TOGGLE ----------------------------- */
function Toggle({ on, onChange }) {
  return (
    <button onClick={() => onChange(!on)} style={{
      width: 52, height: 30, borderRadius: 99, padding: 3, flexShrink: 0,
      background: on ? "linear-gradient(135deg, var(--accent-2), var(--accent-strong))" : "var(--glass-3)",
      border: `1px solid ${on ? "transparent" : "var(--glass-border)"}`,
      boxShadow: on ? "inset 0 1px 0 rgba(255,255,255,.4)" : "inset 0 1px 3px rgba(0,0,0,.2)",
      transition: "all .4s var(--ease)", display: "flex",
      justifyContent: on ? "flex-end" : "flex-start",
    }}>
      <span style={{
        width: 24, height: 24, borderRadius: "50%", background: "#fff",
        boxShadow: "0 2px 6px rgba(0,0,0,.3)", transition: "all .4s var(--ease)",
      }} />
    </button>
  );
}

/* ----------------------------- SEGMENTED ----------------------------- */
function Segmented({ options, value, onChange, size = "md" }) {
  const h = size === "sm" ? 36 : 44;
  return (
    <div className="row" style={{
      gap: 4, padding: 4, borderRadius: "var(--r-pill)", background: "var(--field)",
      border: "1px solid var(--glass-border)", boxShadow: "inset 0 1px 0 var(--glass-hi)",
    }}>
      {options.map((o) => {
        const active = o.value === value;
        return (
          <button key={o.value} onClick={() => onChange(o.value)} style={{
            display: "inline-flex", alignItems: "center", gap: 7, height: h, padding: "0 16px",
            borderRadius: "var(--r-pill)", fontSize: 14.5, fontWeight: 600,
            color: active ? "var(--accent-ink)" : "var(--text-2)",
            background: active ? "linear-gradient(135deg, var(--accent-2), var(--accent-strong))" : "transparent",
            boxShadow: active ? "0 6px 16px -8px var(--accent-strong)" : "none",
            transition: "all .35s var(--ease)",
          }}>
            {o.icon && <Icon name={o.icon} size={17} />}
            {o.label}
          </button>
        );
      })}
    </div>
  );
}

/* ----------------------------- AVATAR ----------------------------- */
const AV_GRAD = [
  ["#a855f7", "#6d28d9"], ["#ec4899", "#7c3aed"], ["#8b5cf6", "#2563eb"],
  ["#f472b6", "#9333ea"], ["#c084fc", "#4f46e5"], ["#e879f9", "#7e22ce"],
  ["#a78bfa", "#db2777"], ["#818cf8", "#9333ea"],
];
function seedNum(s) { let n = 0; for (let i = 0; i < (s || "").length; i++) n = (n * 31 + s.charCodeAt(i)) >>> 0; return n; }
function initials(name) {
  const p = (name || "?").trim().split(/\s+/);
  return ((p[0]?.[0] || "") + (p[1]?.[0] || "")).toUpperCase() || "?";
}
function Avatar({ name, src, size = 44, ring }) {
  const g = AV_GRAD[seedNum(name) % AV_GRAD.length];
  return (
    <div style={{
      width: size, height: size, borderRadius: "50%", flexShrink: 0, position: "relative",
      background: src ? `center/cover url(${src})` : `linear-gradient(140deg, ${g[0]}, ${g[1]})`,
      display: "grid", placeItems: "center", color: "#fff",
      fontWeight: 700, fontSize: size * 0.4, letterSpacing: ".01em",
      boxShadow: ring ? "0 0 0 3px var(--bg), 0 0 0 4.5px var(--accent-line)" : "inset 0 1px 0 rgba(255,255,255,.35)",
    }}>
      {!src && initials(name)}
    </div>
  );
}

/* ----------------------------- BADGE / STATUS ----------------------------- */
const STATUS_COLOR = {
  active: ["hsl(150 65% 45%)", "hsl(150 60% 45% / .15)"],
  suspended: ["hsl(350 80% 62%)", "hsl(350 75% 60% / .15)"],
  invited: ["hsl(40 90% 58%)", "hsl(40 85% 55% / .15)"],
};
function Badge({ children, color = "var(--text-2)", bg = "var(--glass-2)", dot, style }) {
  return (
    <span className="row" style={{
      gap: 6, padding: "5px 11px 5px 9px", borderRadius: "var(--r-pill)", fontSize: 13, fontWeight: 600,
      color, background: bg, border: "1px solid var(--glass-border)", whiteSpace: "nowrap", ...style,
    }}>
      {dot && <span style={{ width: 7, height: 7, borderRadius: "50%", background: color, boxShadow: `0 0 8px ${color}` }} />}
      {children}
    </span>
  );
}

/* ----------------------------- SECTION CARD ----------------------------- */
function Panel({ children, className = "", pad = 26, style, hover }) {
  const ref = useRef(null);
  return (
    <div ref={ref} className={`glass ${className}`} style={{ padding: pad, borderRadius: "var(--r-lg)", ...style }}
      onMouseMove={hover ? (e) => {
        const r = ref.current.getBoundingClientRect();
        ref.current.style.setProperty("--mx", `${e.clientX - r.left}px`);
        ref.current.style.setProperty("--my", `${e.clientY - r.top}px`);
      } : undefined}>
      {children}
    </div>
  );
}

/* ----------------------------- ROW ITEM ----------------------------- */
function ListRow({ icon, iconColor, title, sub, right, onClick, accent }) {
  return (
    <div onClick={onClick} className="row" style={{
      gap: 14, padding: "14px 16px", borderRadius: "var(--r-sm)", cursor: onClick ? "pointer" : "default",
      transition: "background .3s var(--ease)",
    }}
      onMouseEnter={(e) => onClick && (e.currentTarget.style.background = "var(--glass-2)")}
      onMouseLeave={(e) => onClick && (e.currentTarget.style.background = "transparent")}>
      {icon && (
        <div style={{
          width: 42, height: 42, borderRadius: 13, display: "grid", placeItems: "center", flexShrink: 0,
          background: accent ? "var(--accent-soft)" : "var(--glass-2)",
          color: iconColor || (accent ? "var(--accent)" : "var(--text-2)"),
          border: "1px solid var(--glass-border)",
        }}>
          <Icon name={icon} size={20} />
        </div>
      )}
      <div className="col" style={{ gap: 2, flex: 1, minWidth: 0 }}>
        <span style={{ fontSize: 15.5, fontWeight: 600, color: "var(--text)" }}>{title}</span>
        {sub && <span style={{ fontSize: 13.5, color: "var(--text-3)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{sub}</span>}
      </div>
      {right}
    </div>
  );
}

/* ----------------------------- THEME / LANG SWITCH ----------------------------- */
function ThemeSwitch({ theme, setTheme }) {
  const dark = theme === "dark";
  return (
    <button title="Тема / Theme" onClick={() => setTheme(dark ? "light" : "dark")} style={{
      width: 44, height: 44, borderRadius: "50%", display: "grid", placeItems: "center",
      background: "var(--glass-2)", border: "1px solid var(--glass-border)", color: "var(--text-2)",
      backdropFilter: "blur(18px)", boxShadow: "inset 0 1px 0 var(--glass-hi)", overflow: "hidden",
      transition: "all .35s var(--ease)",
    }}
      onMouseEnter={(e) => { e.currentTarget.style.color = "var(--accent)"; e.currentTarget.style.transform = "translateY(-2px)"; }}
      onMouseLeave={(e) => { e.currentTarget.style.color = "var(--text-2)"; e.currentTarget.style.transform = "none"; }}>
      <div style={{ position: "relative", width: 20, height: 20 }}>
        <Icon name="sun" size={20} style={{ position: "absolute", inset: 0, transition: "all .5s var(--ease)", opacity: dark ? 0 : 1, transform: dark ? "rotate(-90deg) scale(.4)" : "none" }} />
        <Icon name="moon" size={20} style={{ position: "absolute", inset: 0, transition: "all .5s var(--ease)", opacity: dark ? 1 : 0, transform: dark ? "none" : "rotate(90deg) scale(.4)" }} />
      </div>
    </button>
  );
}

function LangSwitch({ lang, setLang }) {
  return (
    <button title="Язык / Language" onClick={() => setLang(lang === "ru" ? "en" : "ru")} className="row" style={{
      gap: 7, height: 44, padding: "0 14px", borderRadius: "var(--r-pill)",
      background: "var(--glass-2)", border: "1px solid var(--glass-border)", color: "var(--text-2)",
      backdropFilter: "blur(18px)", boxShadow: "inset 0 1px 0 var(--glass-hi)", fontWeight: 700, fontSize: 13.5, letterSpacing: ".06em",
      transition: "all .35s var(--ease)",
    }}
      onMouseEnter={(e) => { e.currentTarget.style.color = "var(--accent)"; e.currentTarget.style.transform = "translateY(-2px)"; }}
      onMouseLeave={(e) => { e.currentTarget.style.color = "var(--text-2)"; e.currentTarget.style.transform = "none"; }}>
      <Icon name="globe" size={17} />
      {lang.toUpperCase()}
    </button>
  );
}

/* ----------------------------- TOP NAV ----------------------------- */
function NavBar({ t, theme, setTheme, lang, setLang, go, route, user, links }) {
  const [scrolled, setScrolled] = useState(false);
  useEffect(() => {
    const el = document.scrollingElement || document.documentElement;
    const onScroll = () => setScrolled((window.scrollY || el.scrollTop) > 12);
    window.addEventListener("scroll", onScroll, true);
    return () => window.removeEventListener("scroll", onScroll, true);
  }, []);
  return (
    <div style={{ position: "fixed", top: 0, left: 0, right: 0, zIndex: 50, display: "grid", placeItems: "center", padding: "16px 20px", pointerEvents: "none" }}>
      <div className="row" style={{
        width: "100%", maxWidth: 1180, justifyContent: "space-between", gap: 16, pointerEvents: "auto",
        padding: "10px 12px 10px 20px", borderRadius: "var(--r-pill)",
        background: scrolled ? "var(--glass-2)" : "var(--glass)",
        border: "1px solid var(--glass-border)", backdropFilter: "blur(28px) saturate(1.6)",
        boxShadow: scrolled ? "var(--glass-shadow), inset 0 1px 0 var(--glass-hi)" : "inset 0 1px 0 var(--glass-hi)",
        transition: "all .5s var(--ease)",
      }}>
        <Logo size={28} onClick={() => go("landing")} />
        {links && (
          <div className="row nav-links" style={{ gap: 4 }}>
            {links.map((l) => (
              <a key={l.id} onClick={() => go(l.to)} style={{
                padding: "9px 16px", borderRadius: "var(--r-pill)", fontSize: 14.5, fontWeight: 500,
                color: "var(--text-2)", cursor: "pointer", transition: "all .3s",
              }}
                onMouseEnter={(e) => { e.currentTarget.style.background = "var(--glass-2)"; e.currentTarget.style.color = "var(--text)"; }}
                onMouseLeave={(e) => { e.currentTarget.style.background = "transparent"; e.currentTarget.style.color = "var(--text-2)"; }}>
                {l.label}
              </a>
            ))}
          </div>
        )}
        <div className="row" style={{ gap: 9 }}>
          <LangSwitch lang={lang} setLang={setLang} />
          <ThemeSwitch theme={theme} setTheme={setTheme} />
          {user ? (
            <button onClick={() => go("account")} className="row" style={{ gap: 9, padding: "5px 7px 5px 14px", borderRadius: "var(--r-pill)", background: "var(--glass-2)", border: "1px solid var(--glass-border)", color: "var(--text)", fontWeight: 600, fontSize: 14.5 }}>
              <span className="nav-links">{user.name.split(" ")[0]}</span>
              <Avatar name={user.name} src={user.avatar} size={34} />
            </button>
          ) : (
            <>
              <button onClick={() => go("login")} className="nav-login" style={{ padding: "0 18px", height: 44, borderRadius: "var(--r-pill)", color: "var(--text)", fontWeight: 600, fontSize: 14.5 }}>{t.nav_login}</button>
              <Button size="md" onClick={() => go("signup")} style={{ height: 44 }}>{t.nav_signup}</Button>
            </>
          )}
        </div>
      </div>
    </div>
  );
}

Object.assign(window, {
  Icon, ICONS, Logo, Button, IconBtn, Field, Toggle, Segmented,
  Avatar, AV_GRAD, seedNum, initials, Badge, STATUS_COLOR, Panel, ListRow,
  ThemeSwitch, LangSwitch, NavBar,
});
