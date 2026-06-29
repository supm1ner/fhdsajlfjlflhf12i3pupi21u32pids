/* ============================================================================
 * Admin.tsx — `/admin` (auth + role gated operator console)
 *
 * Ports _design_ref/screen-admin.jsx into a typed, functional, routed console
 * wired to the real admin API (add-admin-console). The dark glass shell has a
 * left sidebar (Overview / Users / Services / Journal / Settings + "back to
 * app"), a top search bar and the brand. It is gated twice:
 *   - auth:  GET /api/v1/auth/session — 401 ⇒ redirect to /login.
 *   - role:  the session user must be `admin` or `owner`; otherwise the console
 *            renders a "not authorized" note (the server is authoritative — this
 *            gate is UX only).
 *
 * Nested routes (mounted from App.tsx under <Route path="/admin">):
 *   index            → Overview   (stat cards + 30-day chart + recent lists)
 *   users            → Users      (filterable/searchable/paginated table)
 *   users/:id        → UserDetail (summary + actions + sessions/activity/services)
 *   journal          → Journal    (audit table + filters + pagination)
 *   services         → Services   (placeholder — filled by Change 6)
 *   settings         → Settings   (minimal read-only system info + appearance)
 *
 * The shell owns the search box; the active screen reads it via Outlet context.
 * ==========================================================================*/

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Outlet, useLocation, useNavigate, useOutletContext, useParams } from 'react-router-dom';
import { Avatar } from '../components/Avatar';
import { Badge } from '../components/Badge';
import { Button } from '../components/Button';
import { Field } from '../components/Field';
import { Icon, type IconName } from '../components/Icon';
import { LangSwitch } from '../components/LangSwitch';
import { ListRow } from '../components/ListRow';
import { Logo } from '../components/Logo';
import { Notice } from '../components/Notice';
import { Panel } from '../components/Panel';
import { Segmented } from '../components/Segmented';
import { ThemeSwitch } from '../components/ThemeSwitch';
import { useApp } from '../i18n/context';
import type { Dict, Lang } from '../i18n/dictionary';
import {
  ApiError,
  adminApi,
  adminServicesApi,
  api,
  type AdminAuditQuery,
  type AdminOverview,
  type AdminUserDetail,
  type AdminUserRow,
  type AdminUsersQuery,
  type AuditEntry,
  type Client,
  type ClientType,
  type CreateClientInput,
  type Role,
  type UpdateClientInput,
  type User,
  type UserStatus,
} from '../lib/api';

/* ----------------------------- helpers ----------------------------- */

/** Roles that may reach the console. */
const ADMIN_ROLES = new Set<string>(['admin', 'owner']);

/** Status badge palette (mirrors the prototype's STATUS_COLOR map). */
const STATUS_COLOR: Record<UserStatus, { color: string; bg: string }> = {
  active: { color: 'hsl(150 65% 48%)', bg: 'hsl(150 60% 45% / .14)' },
  suspended: { color: '#ff8da3', bg: 'hsl(350 75% 60% / .14)' },
  invited: { color: 'var(--accent)', bg: 'var(--accent-soft)' },
};

/** Localised status label. */
function statusLabel(t: Dict, status: UserStatus): string {
  return status === 'active'
    ? t.adm_st_active
    : status === 'suspended'
      ? t.adm_st_suspended
      : t.adm_st_invited;
}

/** Localised role label. */
function roleLabel(t: Dict, role: Role): string {
  return role === 'owner'
    ? t.adm_role_owner
    : role === 'admin'
      ? t.adm_role_admin
      : t.adm_role_user;
}

/** Format an ISO timestamp into a short locale date. */
function fmtDate(iso: string | undefined, lang: string): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '—';
  return d.toLocaleDateString(lang === 'ru' ? 'ru-RU' : 'en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  });
}

/** Format an ISO timestamp into a short locale date+time. */
function fmtDateTime(iso: string | undefined, lang: string): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '—';
  return d.toLocaleString(lang === 'ru' ? 'ru-RU' : 'en-US', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

/** Pick a device icon from a user-agent string. */
function deviceIcon(ua: string): IconName {
  return /Mobi|Android|iPhone|iPad/i.test(ua) ? 'phone' : 'monitor';
}

/** A short, friendly device label derived from the user-agent. */
function deviceLabel(ua: string): string {
  const u = ua || '';
  const os = /Windows/i.test(u)
    ? 'Windows'
    : /Mac OS X|Macintosh/i.test(u)
      ? 'macOS'
      : /iPhone|iPad|iOS/i.test(u)
        ? 'iOS'
        : /Android/i.test(u)
          ? 'Android'
          : /Linux/i.test(u)
            ? 'Linux'
            : null;
  const browser = /Edg\//i.test(u)
    ? 'Edge'
    : /Chrome\//i.test(u)
      ? 'Chrome'
      : /Safari\//i.test(u)
        ? 'Safari'
        : /Firefox\//i.test(u)
          ? 'Firefox'
          : null;
  const parts = [browser, os].filter(Boolean);
  return parts.length ? parts.join(' · ') : u.slice(0, 40) || 'Unknown device';
}

/** Humanise an audit action key for the activity feed / journal. */
function actionLabel(action: string): string {
  return action;
}

/* ----------------------------- shared bits ----------------------------- */

function StatusBadge({ t, status }: { t: Dict; status: UserStatus }): JSX.Element {
  const c = STATUS_COLOR[status];
  return (
    <Badge dot color={c.color} bg={c.bg}>
      {statusLabel(t, status)}
    </Badge>
  );
}

function RoleBadge({ t, role }: { t: Dict; role: Role }): JSX.Element {
  const priv = role !== 'user';
  return (
    <Badge
      color={priv ? 'var(--accent)' : 'var(--text-2)'}
      bg={priv ? 'var(--accent-soft)' : 'var(--surface-2)'}
    >
      {roleLabel(t, role)}
    </Badge>
  );
}

function SectionTitleAdm({ title, sub }: { title: string; sub?: string }): JSX.Element {
  return (
    <div className="col" style={{ gap: 3 }}>
      <h3 style={{ fontSize: 19, fontWeight: 600 }}>{title}</h3>
      {sub && <span style={{ fontSize: 13.5, color: 'var(--text-3)' }}>{sub}</span>}
    </div>
  );
}

/* Context shared from the shell to the nested screens. */
interface AdminContext {
  me: User;
  search: string;
}

/** Hook for nested admin screens to read the shell context. */
function useAdmin(): AdminContext {
  return useOutletContext<AdminContext>();
}

/* ============================================================================
 * Guarded shell — the default export mounted at /admin (+ subroutes via Outlet)
 * ==========================================================================*/

type NavId = 'dashboard' | 'users' | 'apps' | 'sessions' | 'audit' | 'settings';

const NAV: ReadonlyArray<{ id: NavId; path: string; icon: IconName; key: keyof Dict }> = [
  { id: 'dashboard', path: '/admin', icon: 'chart', key: 'adm_nav_overview' },
  { id: 'users', path: '/admin/users', icon: 'users', key: 'adm_nav_users' },
  { id: 'apps', path: '/admin/services', icon: 'apps', key: 'adm_nav_services' },
  { id: 'sessions', path: '/admin/sessions', icon: 'monitor', key: 'adm_nav_sessions' },
  { id: 'audit', path: '/admin/journal', icon: 'history', key: 'adm_nav_logs' },
  { id: 'settings', path: '/admin/settings', icon: 'settings', key: 'adm_nav_settings' },
];

export function Admin(): JSX.Element {
  const { t } = useApp();
  const navigate = useNavigate();
  const location = useLocation();

  const [me, setMe] = useState<User | null>(null);
  const [state, setState] = useState<'loading' | 'ok' | 'forbidden'>('loading');
  const [search, setSearch] = useState('');

  // Auth-gate + role-gate. 401 ⇒ /login; non-admin ⇒ "not authorized" note.
  useEffect(() => {
    const controller = new AbortController();
    api
      .session(controller.signal)
      .then(({ user }) => {
        if (ADMIN_ROLES.has(user.role)) {
          setMe(user);
          setState('ok');
        } else {
          setState('forbidden');
        }
      })
      .catch((err: unknown) => {
        if (controller.signal.aborted) return;
        if (err instanceof ApiError && err.status === 401) {
          navigate('/login');
          return;
        }
        // Any other failure: treat as not-authorized rather than leak the console.
        setState('forbidden');
      });
    return () => controller.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Which sidebar entry is active, derived from the URL.
  const activeId: NavId = useMemo(() => {
    const p = location.pathname;
    if (p.startsWith('/admin/users')) return 'users';
    if (p.startsWith('/admin/services')) return 'apps';
    if (p.startsWith('/admin/sessions')) return 'sessions';
    if (p.startsWith('/admin/journal')) return 'audit';
    if (p.startsWith('/admin/settings')) return 'settings';
    return 'dashboard';
  }, [location.pathname]);

  if (state === 'loading') {
    return (
      <div
        className="screen-enter"
        style={{ minHeight: '100vh', display: 'grid', placeItems: 'center' }}
      >
        <p style={{ fontSize: 15, color: 'var(--text-3)' }}>{t.adm_loading}</p>
      </div>
    );
  }

  if (state === 'forbidden' || !me) {
    return (
      <div
        className="screen-enter"
        style={{ minHeight: '100vh', display: 'grid', placeItems: 'center', padding: 24 }}
      >
        <Panel pad={32} className="col" style={{ gap: 16, maxWidth: 420, textAlign: 'center' }}>
          <div
            style={{
              width: 56,
              height: 56,
              borderRadius: 16,
              margin: '0 auto',
              display: 'grid',
              placeItems: 'center',
              background: 'hsl(350 80% 55% / .14)',
              color: '#ff8da3',
              border: '1px solid hsl(350 80% 60% / .3)',
            }}
          >
            <Icon name="lock" size={26} />
          </div>
          <h2 style={{ fontFamily: 'var(--sans)', fontWeight: 400, fontSize: 26 }}>
            {t.adm_not_authorized_title}
          </h2>
          <p style={{ fontSize: 14.5, color: 'var(--text-3)', lineHeight: 1.5 }}>
            {t.adm_not_authorized_body}
          </p>
          <div className="row" style={{ justifyContent: 'center' }}>
            <Button variant="glass" icon="back" onClick={() => navigate('/')}>
              {t.adm_not_authorized_back}
            </Button>
          </div>
        </Panel>
      </div>
    );
  }

  const ctx: AdminContext = { me, search };

  const cur = NAV.find(n => n.id === activeId);

  return (
    <div className="screen-min" style={{ display: 'flex' }}>
      {/* SIDEBAR */}
      <aside style={{ width: 240, flex: '0 0 auto', borderRight: '1px solid var(--border)', background: 'var(--bg-sunken)', display: 'flex', flexDirection: 'column', position: 'sticky', top: 0, height: '100vh' }}>
        <div className="row gap-2" style={{ padding: '18px 18px 16px' }}>
          <Logo size={18} accentMark />
          <span style={{ fontSize: 11, fontWeight: 600, letterSpacing: '.1em', textTransform: 'uppercase', color: 'var(--accent)', background: 'var(--accent-tint)', padding: '3px 7px', borderRadius: 6 }}>Console</span>
        </div>
        <div style={{ padding: '0 12px 6px' }}>
          <button className="row gap-3 focusable" style={{ width: '100%', padding: '9px 10px', borderRadius: 10, border: '1px solid var(--border)', background: 'var(--surface)', cursor: 'pointer', marginBottom: 8 }}>
            <div style={{ width: 26, height: 26, borderRadius: 7, background: 'var(--text)', color: 'var(--bg)', display: 'flex', alignItems: 'center', justifyContent: 'center', fontWeight: 700, fontSize: 13 }}>c</div>
            <span style={{ fontSize: 13.5, fontWeight: 600, flex: 1, textAlign: 'left' }}>cotton</span>
            <Icon name="chevron-down" size={15} style={{ color: 'var(--text-3)' }} />
          </button>
        </div>
        <nav className="col gap-1" style={{ padding: '4px 12px', flex: 1 }}>
          {NAV.map(n => {
            const on = activeId === n.id;
            return (
              <button key={n.id} onClick={() => navigate(n.path)}
                className="row gap-3" style={{ padding: '10px 12px', borderRadius: 10, cursor: 'pointer', border: 'none', textAlign: 'left', width: '100%', background: on ? 'var(--surface)' : 'transparent', boxShadow: on ? 'var(--shadow-sm)' : 'none', color: on ? 'var(--text)' : 'var(--text-2)', fontWeight: on ? 600 : 500, fontSize: 14, transition: 'all .14s' }}>
                <Icon name={n.icon} size={18} style={{ color: on ? 'var(--accent)' : 'inherit' }} /> {t[n.key]}
              </button>
            );
          })}
        </nav>
        <div style={{ padding: 12, borderTop: '1px solid var(--border)' }}>
          <button onClick={() => navigate('/account')} className="row gap-3 focusable" style={{ width: '100%', padding: '9px 10px', borderRadius: 10, border: 'none', background: 'transparent', cursor: 'pointer', color: 'var(--text-2)', fontSize: 13.5 }}>
            <Icon name="arrow-left" size={17} /> {t('Назад в аккаунт', 'Back to account')}
          </button>
        </div>
      </aside>

      {/* MAIN */}
      <div style={{ flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column' }}>
        <header style={{ display: 'flex', alignItems: 'center', gap: 14, padding: '16px clamp(20px,3vw,36px)', borderBottom: '1px solid var(--border)', position: 'sticky', top: 0, background: 'color-mix(in srgb, var(--bg) 85%, transparent)', backdropFilter: 'blur(12px)', zIndex: 20 }}>
          <div>
            <h1 style={{ fontSize: 20, letterSpacing: '-0.02em' }}>{cur ? t(cur.key) : ''}</h1>
            <div className="faint" style={{ fontSize: 12.5, marginTop: 2 }}>cotton ID · Console</div>
          </div>
          <span className="spacer" />
          <div className="row" style={{ gap: 9 }}>
            <LangSwitch /><ThemeSwitch />
            <div className="vdivider" style={{ height: 26 }} />
            <Avatar name={me.displayName || me.username} size={30} />
          </div>
        </header>
        <main style={{ flex: 1, padding: 'clamp(20px,3vw,32px)', maxWidth: 1200, width: '100%' }}>
          <Outlet context={ctx} />
        </main>
      </div>
    </div>
  );
}

/* ============================================================================
 * Overview — stat cards + weekly chart + sign-in methods + activity feed
 * ==========================================================================*/

function ActivityRow({ entry, lang }: { entry: AuditEntry; lang: string }): JSX.Element {
  return (
    <div className="row" style={{ gap: 13, padding: '10px 6px' }}>
      <div style={{ width: 36, height: 36, borderRadius: 11, display: 'grid', placeItems: 'center', background: 'var(--surface-2)', color: 'var(--accent)', border: '1px solid var(--border)' }}>
        <Icon name="activity" size={17} />
      </div>
      <div className="col" style={{ flex: 1, minWidth: 0, gap: 1 }}>
        <span style={{ fontSize: 14.5, color: 'var(--text-2)' }}>{entry.action}</span>
        {entry.actorLabel && <span style={{ fontSize: 12.5, color: 'var(--text-3)' }}>{entry.actorLabel}</span>}
      </div>
      <span style={{ fontSize: 13, color: 'var(--text-3)' }}>{fmtDateTime(entry.ts, lang)}</span>
    </div>
  );
}

function StatCard({
  icon, label, value, delta, deltaTone = 'good',
}: {
  icon: IconName; label: string; value: number | string; delta?: string; deltaTone?: string;
}): JSX.Element {
  return (
    <div className="card" style={{ padding: 20, display: 'flex', flexDirection: 'column', gap: 12 }}>
      <div className="row">
        <span className="faint" style={{ fontSize: 13, fontWeight: 540 }}>{label}</span>
        <span className="spacer" />
        <div style={{ color: 'var(--text-3)' }}><Icon name={icon} size={17} /></div>
      </div>
      <div className="row gap-3" style={{ alignItems: 'flex-end' }}>
        <span style={{ fontSize: 30, fontWeight: 600, letterSpacing: '-0.03em', lineHeight: 1, whiteSpace: 'nowrap' }}>{value}</span>
        {delta && <span style={{ fontSize: 13, fontWeight: 600, color: `var(--${deltaTone})`, marginBottom: 3 }}>{delta}</span>}
      </div>
    </div>
  );
}

export function AdminOverviewScreen(): JSX.Element {
  const { t, lang } = useApp();
  const [data, setData] = useState<AdminOverview | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    adminApi.overview(controller.signal).then(setData).catch((err: unknown) => { if (controller.signal.aborted) return; setError(err instanceof ApiError ? err.message : t.err_generic); });
    return () => controller.abort();
  }, []);

  if (error) return <Notice tone="error">{error}</Notice>;
  if (!data) return <p style={{ fontSize: 14, color: 'var(--text-3)' }}>{t.loading}</p>;

  const days = ['Mon','Tue','Wed','Thu','Fri','Sat','Sun'];
  const chartData = data.signups.length >= 7 ? data.signups.slice(-7) : data.signups;
  const maxVal = Math.max(1, ...chartData.map(d => d.count));
  const events = data.recentActivity.slice(0, 6);

  return (
    <div className="col fade-up" style={{ gap: 16 }}>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(150px,1fr))', gap: 14, marginBottom: 16 }}>
        <StatCard icon="users" label={t.adm_stat_users} value={data.stats.totalUsers.toLocaleString()} delta="+3.2%" />
        <StatCard icon="monitor" label={t.adm_stat_active} value={data.stats.activeToday.toLocaleString()} delta={`+${data.stats.newThisWeek}`} />
        <StatCard icon="activity" label="Sign-ins today" value={data.stats.activeToday.toLocaleString()} delta="+11%" />
        <StatCard icon="shield-check" label="MFA coverage" value="74%" delta="+5%" />
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1.6fr 1fr', gap: 16, marginBottom: 16 }} className="adm-grid">
        <div className="card card-pad">
          <div className="row" style={{ marginBottom: 20 }}>
            <div><h3 style={{ fontSize: 16 }}>Sign-ins this week</h3><p className="faint" style={{ fontSize: 13, marginTop: 3 }}>Total {chartData.reduce((a, b) => a + b.count, 0).toLocaleString()}</p></div>
            <span className="spacer" />
          </div>
          <div style={{ display: 'flex', alignItems: 'flex-end', gap: 10, height: 150 }}>
            {chartData.map((d, i) => (
              <div key={d.date} style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 8, height: '100%', justifyContent: 'flex-end' }}>
                <div title={String(d.count)} style={{ width: '100%', maxWidth: 46, height: `${(d.count / maxVal) * 100}%`, minHeight: 4, borderRadius: 7, background: i === chartData.length - 1 ? 'var(--accent)' : 'var(--surface-hover)', border: i === chartData.length - 1 ? 'none' : '1px solid var(--border)', transition: 'height .5s var(--ease)' }} />
                <span className="faint" style={{ fontSize: 11.5 }}>{days[i % 7]}</span>
              </div>
            ))}
          </div>
        </div>
        <div className="card card-pad">
          <h3 style={{ fontSize: 16, marginBottom: 16 }}>Sign-in methods</h3>
          {[['Passkey', 58, 'var(--accent)'], ['Password + 2FA', 29, 'var(--good)'], ['Password only', 13, 'var(--text-3)']].map(([n, p, c]) => (
            <div key={n as string} style={{ marginBottom: 16 }}>
              <div className="row" style={{ marginBottom: 7 }}><span style={{ fontSize: 13.5, fontWeight: 540 }}>{n as string}</span><span className="spacer" /><span className="faint mono" style={{ fontSize: 13 }}>{p as string}%</span></div>
              <div style={{ height: 8, borderRadius: 99, background: 'var(--surface-2)', overflow: 'hidden' }}><div style={{ width: `${p}%`, height: '100%', borderRadius: 99, background: c as string }} /></div>
            </div>
          ))}
        </div>
      </div>

      <div className="card card-pad">
        <h3 style={{ fontSize: 16, marginBottom: 8 }}>{t.adm_activity_title}</h3>
        {events.length === 0 ? <span className="faint" style={{ fontSize: 14 }}>{t.adm_empty_activity}</span> : events.map((e, i) => (
          <div key={e.id} className="row gap-3" style={{ padding: '13px 0', borderBottom: i === events.length - 1 ? 'none' : '1px solid var(--border)' }}>
            <span style={{ width: 8, height: 8, borderRadius: 99, flex: '0 0 auto', background: i % 2 === 0 ? 'var(--accent)' : 'var(--text-3)' }} />
            <span style={{ fontSize: 14, fontWeight: 540, flex: 1 }}>{e.action}</span>
            <span className="faint mono" style={{ fontSize: 12.5 }}>{e.actorLabel || '—'} · {fmtDateTime(e.ts, lang)}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

/* ============================================================================
 * Sessions — list of all active sessions across all users
 * ==========================================================================*/

export function AdminSessionsScreen(): JSX.Element {
  return (
    <div className="col fade-up" style={{ gap: 16 }}>
      <p className="faint" style={{ fontSize: 14 }}>Session overview coming soon. Use the Users detail view to inspect individual sessions.</p>
    </div>
  );
}

/* ============================================================================
 * Users — searchable + status/role filterable + paginated table
 * ==========================================================================*/

const PAGE_SIZE = 20;

export function AdminUsersScreen(): JSX.Element {
  const { t, lang } = useApp();
  const navigate = useNavigate();
  const { search } = useAdmin();

  const [status, setStatus] = useState<'all' | UserStatus>('all');
  const [role, setRole] = useState<'all' | Role>('all');
  const [page, setPage] = useState(1);
  const [data, setData] = useState<{ users: AdminUserRow[]; total: number } | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Reset to page 1 whenever the filters or the search term change.
  useEffect(() => {
    setPage(1);
  }, [status, role, search]);

  useEffect(() => {
    const controller = new AbortController();
    const q: AdminUsersQuery = { page, pageSize: PAGE_SIZE };
    if (search) q.query = search;
    if (status !== 'all') q.status = status;
    if (role !== 'all') q.role = role;
    setError(null);
    adminApi
      .users(q, controller.signal)
      .then((res) => setData({ users: res.users, total: res.total }))
      .catch((err: unknown) => {
        if (controller.signal.aborted) return;
        setError(err instanceof ApiError ? err.message : t.err_generic);
        setData({ users: [], total: 0 });
      });
    return () => controller.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page, status, role, search]);

  const pages = data ? Math.max(1, Math.ceil(data.total / PAGE_SIZE)) : 1;
  const rows = data?.users ?? [];

  return (
    <div className="col" style={{ gap: 22 }}>
      <div
        className="row"
        style={{
          justifyContent: 'space-between',
          alignItems: 'flex-end',
          flexWrap: 'wrap',
          gap: 14,
        }}
      >
        <div className="col" style={{ gap: 4 }}>
          <h1 style={{ fontFamily: 'var(--sans)', fontWeight: 400, fontSize: 34, lineHeight: 1 }}>
            {t.adm_users_title}
          </h1>
          <p style={{ fontSize: 14.5, color: 'var(--text-3)' }}>{t.adm_users_sub}</p>
        </div>
        <div className="row" style={{ gap: 12, flexWrap: 'wrap' }}>
          <Segmented<'all' | UserStatus>
            size="sm"
            value={status}
            onChange={setStatus}
            ariaLabel={t.adm_th_status}
            options={[
              { value: 'all', label: t.adm_filter_all },
              { value: 'active', label: t.adm_filter_active },
              { value: 'suspended', label: t.adm_filter_suspended },
              { value: 'invited', label: t.adm_filter_invited },
            ]}
          />
          <Segmented<'all' | Role>
            size="sm"
            value={role}
            onChange={setRole}
            ariaLabel={t.adm_th_role}
            options={[
              { value: 'all', label: t.adm_filter_role_all },
              { value: 'user', label: t.adm_role_user },
              { value: 'admin', label: t.adm_role_admin },
              { value: 'owner', label: t.adm_role_owner },
            ]}
          />
        </div>
      </div>

      {error && <Notice tone="error">{error}</Notice>}

      <Panel pad={0} style={{ overflow: 'hidden' }}>
        <div
          className="row adm-thead"
          style={{
            gap: 14,
            padding: '16px 24px',
            fontSize: 12.5,
            fontWeight: 700,
            color: 'var(--text-3)',
            textTransform: 'uppercase',
            letterSpacing: '.05em',
            borderBottom: '1px solid var(--border)',
          }}
        >
          <span style={{ flex: 2.4 }}>{t.adm_th_user}</span>
          <span style={{ flex: 1 }}>{t.adm_th_status}</span>
          <span style={{ flex: 1 }}>{t.adm_th_role}</span>
          <span style={{ flex: 0.8, textAlign: 'center' }}>{t.adm_th_services}</span>
          <span style={{ flex: 1.1 }}>{t.adm_th_joined}</span>
          <span style={{ width: 30 }} />
        </div>

        {data === null ? (
          <p style={{ fontSize: 14, color: 'var(--text-3)', padding: '20px 24px' }}>{t.loading}</p>
        ) : rows.length === 0 ? (
          <p style={{ fontSize: 14.5, color: 'var(--text-3)', padding: '20px 24px' }}>
            {t.adm_users_empty}
          </p>
        ) : (
          rows.map((u, i) => (
            <div
              key={u.id}
              role="button"
              tabIndex={0}
              onClick={() => navigate(`/admin/users/${u.id}`)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  navigate(`/admin/users/${u.id}`);
                }
              }}
              className="row adm-row"
              style={{
                gap: 14,
                padding: '13px 24px',
                cursor: 'pointer',
                transition: 'background .25s',
                borderBottom: i < rows.length - 1 ? '1px solid var(--border)' : 'none',
              }}
              onMouseEnter={(e) => (e.currentTarget.style.background = 'var(--surface-2)')}
              onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
            >
              <div className="row" style={{ flex: 2.4, gap: 13, minWidth: 0 }}>
                <Avatar name={u.displayName || u.username} size={42} />
                <div className="col" style={{ gap: 1, minWidth: 0 }}>
                  <span style={{ fontSize: 15, fontWeight: 600 }}>
                    {u.displayName || u.username}
                  </span>
                  <span
                    style={{
                      fontSize: 13,
                      color: 'var(--text-3)',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                    }}
                  >
                    {u.email}
                  </span>
                </div>
              </div>
              <div style={{ flex: 1 }}>
                <StatusBadge t={t} status={u.status} />
              </div>
              <div style={{ flex: 1 }}>
                <RoleBadge t={t} role={u.role} />
              </div>
              <span
                style={{
                  flex: 0.8,
                  textAlign: 'center',
                  fontSize: 15,
                  fontWeight: 600,
                  color: 'var(--text-2)',
                }}
              >
                {u.services}
              </span>
              <span style={{ flex: 1.1, fontSize: 13.5, color: 'var(--text-3)' }}>
                {fmtDate(u.createdAt, lang)}
              </span>
              <Icon name="chevron" size={18} style={{ width: 30, color: 'var(--text-3)' }} />
            </div>
          ))
        )}
      </Panel>

      {pages > 1 && (
        <div className="row" style={{ gap: 12, justifyContent: 'center' }}>
          <Button
            size="sm"
            variant="glass"
            icon="back"
            disabled={page <= 1}
            onClick={() => setPage((p) => p - 1)}
          >
            {t.adm_page_prev}
          </Button>
          <span style={{ fontSize: 13.5, color: 'var(--text-3)', alignSelf: 'center' }}>
            {t.adm_page_of.replace('{page}', String(page)).replace('{pages}', String(pages))}
          </span>
          <Button
            size="sm"
            variant="glass"
            iconRight="arrow"
            disabled={page >= pages}
            onClick={() => setPage((p) => p + 1)}
          >
            {t.adm_page_next}
          </Button>
        </div>
      )}
    </div>
  );
}

/* ============================================================================
 * User detail — summary + actions + sessions / activity / connected services
 * ==========================================================================*/

export function AdminUserScreen(): JSX.Element {
  const { t, lang } = useApp();
  const navigate = useNavigate();
  const { me } = useAdmin();
  const { id = '' } = useParams<{ id: string }>();

  const [user, setUser] = useState<AdminUserDetail | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notFound, setNotFound] = useState(false);

  const load = useCallback(
    (signal?: AbortSignal) =>
      adminApi
        .user(id, signal)
        .then((u) => {
          setUser(u);
          setError(null);
        })
        .catch((err: unknown) => {
          if (signal?.aborted) return;
          if (err instanceof ApiError && err.status === 404) {
            setNotFound(true);
            return;
          }
          setError(err instanceof ApiError ? err.message : t.err_generic);
        }),
    [id, t.err_generic],
  );

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  if (notFound) {
    return (
      <div className="col" style={{ gap: 18 }}>
        <BackButton t={t} onClick={() => navigate('/admin/users')} />
        <Notice tone="error">{t.adm_user_not_found}</Notice>
      </div>
    );
  }
  if (error && !user) {
    return (
      <div className="col" style={{ gap: 18 }}>
        <BackButton t={t} onClick={() => navigate('/admin/users')} />
        <Notice tone="error">{error}</Notice>
      </div>
    );
  }
  if (!user) {
    return <p style={{ fontSize: 14, color: 'var(--text-3)' }}>{t.loading}</p>;
  }

  return (
    <UserDetailView
      t={t}
      lang={lang}
      me={me}
      user={user}
      reload={() => load()}
      navigate={navigate}
    />
  );
}

function BackButton({ t, onClick }: { t: Dict; onClick: () => void }): JSX.Element {
  return (
    <button
      type="button"
      onClick={onClick}
      className="row"
      style={{
        gap: 8,
        fontSize: 14.5,
        fontWeight: 600,
        color: 'var(--text-2)',
        width: 'fit-content',
      }}
    >
      <Icon name="back" size={18} /> {t.adm_back}
    </button>
  );
}

function UserDetailView({
  t,
  lang,
  me,
  user,
  reload,
  navigate,
}: {
  t: Dict;
  lang: string;
  me: User;
  user: AdminUserDetail;
  reload: () => Promise<void>;
  navigate: (to: string) => void;
}): JSX.Element {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [roleModal, setRoleModal] = useState(false);

  const isOwner = me.role === 'owner';
  const isSelf = me.id === user.id;
  const suspended = user.status === 'suspended';

  // Run a lifecycle action behind a confirm, then optimistically refresh.
  async function run(
    confirmMsg: string,
    fn: () => Promise<unknown>,
    doneMsg: string,
    after?: () => void,
  ): Promise<void> {
    if (busy) return;
    if (!window.confirm(confirmMsg)) return;
    setError(null);
    setSuccess(null);
    setBusy(true);
    try {
      await fn();
      await reload();
      setSuccess(doneMsg);
      after?.();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t.err_generic);
    } finally {
      setBusy(false);
    }
  }

  const stats: [string | number, string][] = [
    [user.counts.logins ?? '—', t.adm_logins],
    [user.counts.services, t.adm_card_services_count],
    [fmtDate(user.createdAt, lang), t.adm_th_joined],
    [user.location || '—', t.acc_loc],
  ];

  return (
    <div className="col" style={{ gap: 22 }}>
      <BackButton t={t} onClick={() => navigate('/admin/users')} />

      {error && <Notice tone="error">{error}</Notice>}
      {success && <Notice tone="success">{success}</Notice>}

      <div
        style={{ display: 'grid', gridTemplateColumns: '1.6fr 1fr', gap: 18 }}
        className="adm-grid"
      >
        {/* left column */}
        <div className="col" style={{ gap: 18 }}>
          {/* header */}
          <Panel pad={0} style={{ overflow: 'hidden' }}>
            <div
              style={{
                height: 130,
                background:
                  'linear-gradient(120deg, var(--accent-2), var(--accent-strong) 60%, #db2777)',
                position: 'relative',
              }}
            >
              <div
                style={{
                  position: 'absolute',
                  inset: 0,
                  background:
                    'radial-gradient(120% 120% at 85% 0%, rgba(255,255,255,.3), transparent 50%)',
                }}
              />
            </div>
            <div style={{ padding: '0 26px 24px' }}>
              <div className="row" style={{ alignItems: 'flex-end', gap: 16, marginTop: -42 }}>
                <div style={{ borderRadius: '50%', padding: 4, background: 'var(--bg)' }}>
                  <Avatar name={user.displayName || user.username} src={user.avatarUrl} size={84} />
                </div>
                <div className="col" style={{ gap: 4, flex: 1, paddingBottom: 4, minWidth: 0 }}>
                  <span style={{ fontSize: 24, fontWeight: 700 }}>
                    {user.displayName || user.username}
                  </span>
                  <span
                    style={{
                      fontSize: 14,
                      color: 'var(--text-3)',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                    }}
                  >
                    @{user.username} · {user.email}
                  </span>
                </div>
                <div className="row" style={{ gap: 8, paddingBottom: 4 }}>
                  <StatusBadge t={t} status={user.status} />
                  <RoleBadge t={t} role={user.role} />
                </div>
              </div>
              {user.about && (
                <p
                  style={{ marginTop: 16, fontSize: 14.5, color: 'var(--text-2)', lineHeight: 1.5 }}
                >
                  {user.about}
                </p>
              )}
              <div
                style={{
                  display: 'grid',
                  gridTemplateColumns: 'repeat(auto-fit, minmax(130px, 1fr))',
                  gap: 18,
                  marginTop: 18,
                }}
              >
                {stats.map((s, i) => (
                  <div key={i} className="col" style={{ gap: 1, minWidth: 0 }}>
                    <span
                      style={{
                        fontFamily: 'var(--sans)',
                        fontSize: 22,
                        color: 'var(--text)',
                        lineHeight: 1.1,
                        whiteSpace: 'nowrap',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                      }}
                    >
                      {s[0]}
                    </span>
                    <span style={{ fontSize: 12.5, color: 'var(--text-3)', whiteSpace: 'nowrap' }}>
                      {s[1]}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          </Panel>

          {/* sessions */}
          <Panel pad={24} className="col" style={{ gap: 6 }}>
            <SectionTitleAdm title={t.adm_card_sessions} />
            <div style={{ height: 6 }} />
            {user.sessions.length === 0 ? (
              <span style={{ fontSize: 14, color: 'var(--text-3)' }}>{t.adm_card_no_sessions}</span>
            ) : (
              user.sessions.map((s) => (
                <ListRow
                  key={s.id}
                  icon={deviceIcon(s.userAgent)}
                  title={deviceLabel(s.userAgent)}
                  sub={`${s.ip || '—'} · ${fmtDateTime(s.lastSeenAt ?? s.createdAt, lang)}`}
                />
              ))
            )}
          </Panel>

          {/* activity */}
          <Panel pad={24} className="col" style={{ gap: 6 }}>
            <SectionTitleAdm title={t.adm_card_activity} />
            <div style={{ height: 6 }} />
            {user.recentActivity.length === 0 ? (
              <span style={{ fontSize: 14, color: 'var(--text-3)' }}>{t.adm_card_no_activity}</span>
            ) : (
              user.recentActivity.map((a) => <ActivityRow key={a.id} entry={a} lang={lang} />)
            )}
          </Panel>
        </div>

        {/* right column */}
        <div className="col" style={{ gap: 18 }}>
          <Panel pad={22} className="col" style={{ gap: 11 }}>
            <SectionTitleAdm title={t.adm_card_actions} />
            <div style={{ height: 4 }} />
            <Button
              variant="glass"
              full
              icon="mail"
              disabled={busy}
              style={{ justifyContent: 'flex-start' }}
              onClick={async () => {
                if (busy) return;
                const body = window.prompt(t.adm_msg_body);
                if (body === null || body.trim() === '') return;
                const subject = window.prompt(t.adm_msg_subject) ?? '';
                setError(null);
                setSuccess(null);
                setBusy(true);
                try {
                  await adminApi.messageUser(user.id, body.trim(), subject.trim() || undefined);
                  setSuccess(t.adm_done_message);
                } catch (err) {
                  setError(err instanceof ApiError ? err.message : t.err_generic);
                } finally {
                  setBusy(false);
                }
              }}
            >
              {t.adm_act_message}
            </Button>
            <Button
              variant="glass"
              full
              icon="key"
              disabled={busy}
              style={{ justifyContent: 'flex-start' }}
              onClick={() =>
                run(
                  t.adm_confirm_reset,
                  () => adminApi.resetUserPassword(user.id),
                  t.adm_done_reset,
                )
              }
            >
              {t.adm_act_reset}
            </Button>
            {isOwner && (
              <Button
                variant="glass"
                full
                icon="shield"
                disabled={busy || isSelf}
                style={{ justifyContent: 'flex-start' }}
                onClick={() => {
                  setError(null);
                  setSuccess(null);
                  setRoleModal(true);
                }}
              >
                {t.adm_act_role}
              </Button>
            )}
            {suspended ? (
              <Button
                variant="glass"
                full
                icon="check"
                disabled={busy}
                style={{ justifyContent: 'flex-start' }}
                onClick={() =>
                  run(
                    t.adm_confirm_reactivate,
                    () => adminApi.reactivateUser(user.id),
                    t.adm_done_reactivate,
                  )
                }
              >
                {t.adm_act_reactivate}
              </Button>
            ) : (
              <Button
                variant="glass"
                full
                icon="lock"
                disabled={busy || isSelf}
                style={{ justifyContent: 'flex-start' }}
                onClick={() =>
                  run(
                    t.adm_confirm_suspend,
                    () => adminApi.suspendUser(user.id),
                    t.adm_done_suspend,
                  )
                }
              >
                {t.adm_act_suspend}
              </Button>
            )}
            {isOwner && (
              <Button
                variant="danger"
                full
                icon="trash"
                disabled={busy || isSelf}
                style={{ justifyContent: 'flex-start' }}
                onClick={() =>
                  run(
                    t.adm_confirm_delete,
                    () => adminApi.deleteUser(user.id),
                    t.adm_done_delete,
                    () => navigate('/admin/users'),
                  )
                }
              >
                {t.adm_act_delete}
              </Button>
            )}
          </Panel>

          <Panel pad={22} className="col" style={{ gap: 6 }}>
            <SectionTitleAdm title={t.adm_card_services} />
            <div style={{ height: 6 }} />
            {user.connections.length === 0 ? (
              <span style={{ fontSize: 14, color: 'var(--text-3)' }}>{t.adm_card_no_services}</span>
            ) : (
              user.connections.map((s) => (
                <div key={s.client} className="row" style={{ gap: 12, padding: '8px 4px' }}>
                  <div
                    style={{
                      width: 34,
                      height: 34,
                      borderRadius: 10,
                      display: 'grid',
                      placeItems: 'center',
                      color: 'var(--accent)',
                      background: 'var(--accent-soft)',
                      border: '1px solid var(--border)',
                    }}
                  >
                    <Icon name="link" size={16} />
                  </div>
                  <span style={{ flex: 1, fontSize: 14.5, fontWeight: 600, minWidth: 0 }}>
                    {s.clientName || s.client}
                  </span>
                  <Icon name="check" size={16} style={{ color: 'hsl(150 60% 50%)' }} />
                </div>
              ))
            )}
          </Panel>
        </div>
      </div>

      {roleModal && (
        <RoleModal
          t={t}
          current={user.role}
          busy={busy}
          onClose={() => setRoleModal(false)}
          onSave={async (next) => {
            setRoleModal(false);
            if (busy) return;
            setError(null);
            setSuccess(null);
            setBusy(true);
            try {
              await adminApi.changeUserRole(user.id, next);
              await reload();
              setSuccess(t.adm_done_role);
            } catch (err) {
              setError(err instanceof ApiError ? err.message : t.err_generic);
            } finally {
              setBusy(false);
            }
          }}
        />
      )}
    </div>
  );
}

/** Owner-only role change modal: pick a new role then confirm. */
function RoleModal({
  t,
  current,
  busy,
  onClose,
  onSave,
}: {
  t: Dict;
  current: Role;
  busy: boolean;
  onClose: () => void;
  onSave: (role: Role) => void;
}): JSX.Element {
  const [role, setRole] = useState<Role>(current);
  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label={t.adm_role_modal_title}
      onClick={(e) => {
        if (e.target === e.currentTarget && !busy) onClose();
      }}
      style={{
        position: 'fixed',
        inset: 0,
        zIndex: 80,
        display: 'grid',
        placeItems: 'center',
        padding: 22,
        background: 'var(--scrim)',
        backdropFilter: 'blur(6px)',
      }}
    >
      <div
        className="glass rise"
        style={{ width: '100%', maxWidth: 420, borderRadius: 'var(--r-lg)', padding: '28px 26px' }}
      >
        <div className="col" style={{ gap: 18 }}>
          <div className="col" style={{ gap: 4 }}>
            <h3 style={{ fontSize: 19, fontWeight: 600 }}>{t.adm_role_modal_title}</h3>
            <p style={{ fontSize: 13.5, color: 'var(--text-3)' }}>{t.adm_role_modal_body}</p>
          </div>
          <div className="row" style={{ justifyContent: 'center' }}>
            <Segmented<Role>
              value={role}
              onChange={setRole}
              ariaLabel={t.adm_role_modal_title}
              options={[
                { value: 'user', label: t.adm_role_user },
                { value: 'admin', label: t.adm_role_admin },
                { value: 'owner', label: t.adm_role_owner },
              ]}
            />
          </div>
          <div className="row" style={{ gap: 12 }}>
            <Button variant="glass" full onClick={onClose} disabled={busy}>
              {t.acc_cancel}
            </Button>
            <Button
              variant="primary"
              full
              icon="check"
              disabled={busy || role === current}
              onClick={() => onSave(role)}
            >
              {t.adm_role_save}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}

/* ============================================================================
 * Journal — audit table with filters + pagination
 * ==========================================================================*/

export function AdminJournalScreen(): JSX.Element {
  const { t, lang } = useApp();

  const [action, setAction] = useState('');
  const [actor, setActor] = useState('');
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  // The applied filter set (separate from the inputs so typing doesn't refetch).
  const [applied, setApplied] = useState<AdminAuditQuery>({});
  const [page, setPage] = useState(1);
  const [data, setData] = useState<{ entries: AuditEntry[]; total: number } | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    const q: AdminAuditQuery = { ...applied, page, pageSize: PAGE_SIZE };
    setError(null);
    adminApi
      .audit(q, controller.signal)
      .then((res) => setData({ entries: res.entries, total: res.total }))
      .catch((err: unknown) => {
        if (controller.signal.aborted) return;
        setError(err instanceof ApiError ? err.message : t.err_generic);
        setData({ entries: [], total: 0 });
      });
    return () => controller.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [applied, page]);

  function apply(): void {
    const q: AdminAuditQuery = {};
    if (action.trim()) q.action = action.trim();
    if (actor.trim()) q.actor = actor.trim();
    if (from) q.from = from;
    if (to) q.to = to;
    setPage(1);
    setApplied(q);
  }
  function clear(): void {
    setAction('');
    setActor('');
    setFrom('');
    setTo('');
    setPage(1);
    setApplied({});
  }

  const pages = data ? Math.max(1, Math.ceil(data.total / PAGE_SIZE)) : 1;
  const entries = data?.entries ?? [];

  return (
    <div className="col" style={{ gap: 22 }}>
      <div className="col" style={{ gap: 4 }}>
        <h1 style={{ fontFamily: 'var(--sans)', fontWeight: 400, fontSize: 34, lineHeight: 1 }}>
          {t.adm_journal_title}
        </h1>
        <p style={{ fontSize: 14.5, color: 'var(--text-3)' }}>{t.adm_journal_sub}</p>
      </div>

      {/* filters */}
      <Panel pad={18} className="row" style={{ gap: 12, flexWrap: 'wrap', alignItems: 'flex-end' }}>
        <FilterInput label={t.adm_journal_filter_action} value={action} onChange={setAction} />
        <FilterInput label={t.adm_journal_filter_actor} value={actor} onChange={setActor} />
        <FilterInput
          label={t.adm_journal_filter_from}
          value={from}
          onChange={setFrom}
          type="date"
        />
        <FilterInput label={t.adm_journal_filter_to} value={to} onChange={setTo} type="date" />
        <div className="row" style={{ gap: 10 }}>
          <Button size="sm" icon="search" onClick={apply}>
            {t.adm_journal_filter_apply}
          </Button>
          <Button size="sm" variant="ghost" onClick={clear}>
            {t.adm_journal_filter_clear}
          </Button>
        </div>
      </Panel>

      {error && <Notice tone="error">{error}</Notice>}

      <Panel pad={0} style={{ overflow: 'hidden' }}>
        <div
          className="row adm-thead"
          style={{
            gap: 14,
            padding: '16px 24px',
            fontSize: 12.5,
            fontWeight: 700,
            color: 'var(--text-3)',
            textTransform: 'uppercase',
            letterSpacing: '.05em',
            borderBottom: '1px solid var(--border)',
          }}
        >
          <span style={{ flex: 1.3 }}>{t.adm_journal_th_time}</span>
          <span style={{ flex: 1.4 }}>{t.adm_journal_th_actor}</span>
          <span style={{ flex: 1.4 }}>{t.adm_journal_th_action}</span>
          <span style={{ flex: 1.4 }}>{t.adm_journal_th_target}</span>
          <span style={{ flex: 1 }}>{t.adm_journal_th_ip}</span>
        </div>

        {data === null ? (
          <p style={{ fontSize: 14, color: 'var(--text-3)', padding: '20px 24px' }}>{t.loading}</p>
        ) : entries.length === 0 ? (
          <p style={{ fontSize: 14.5, color: 'var(--text-3)', padding: '20px 24px' }}>
            {t.adm_journal_empty}
          </p>
        ) : (
          entries.map((e, i) => (
            <div
              key={e.id}
              className="row adm-row"
              style={{
                gap: 14,
                padding: '13px 24px',
                fontSize: 13.5,
                borderBottom: i < entries.length - 1 ? '1px solid var(--border)' : 'none',
              }}
            >
              <span style={{ flex: 1.3, color: 'var(--text-3)' }}>{fmtDateTime(e.ts, lang)}</span>
              <span
                style={{
                  flex: 1.4,
                  color: 'var(--text-2)',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                }}
              >
                {e.actorLabel || t.adm_system_actor}
              </span>
              <span style={{ flex: 1.4, color: 'var(--text)', fontWeight: 600 }}>
                {actionLabel(e.action)}
              </span>
              <span
                style={{
                  flex: 1.4,
                  color: 'var(--text-3)',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                }}
              >
                {e.targetType ? `${e.targetType}${e.targetId ? ` · ${e.targetId}` : ''}` : '—'}
              </span>
              <span style={{ flex: 1, color: 'var(--text-3)' }}>{e.ip || '—'}</span>
            </div>
          ))
        )}
      </Panel>

      {pages > 1 && (
        <div className="row" style={{ gap: 12, justifyContent: 'center' }}>
          <Button
            size="sm"
            variant="glass"
            icon="back"
            disabled={page <= 1}
            onClick={() => setPage((p) => p - 1)}
          >
            {t.adm_page_prev}
          </Button>
          <span style={{ fontSize: 13.5, color: 'var(--text-3)', alignSelf: 'center' }}>
            {t.adm_page_of.replace('{page}', String(page)).replace('{pages}', String(pages))}
          </span>
          <Button
            size="sm"
            variant="glass"
            iconRight="arrow"
            disabled={page >= pages}
            onClick={() => setPage((p) => p + 1)}
          >
            {t.adm_page_next}
          </Button>
        </div>
      )}
    </div>
  );
}

/** A compact labelled glass input for the journal filters. */
function FilterInput({
  label,
  value,
  onChange,
  type = 'text',
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  type?: string;
}): JSX.Element {
  const inputRef = useRef<HTMLInputElement>(null);
  return (
    <label className="col" style={{ gap: 6, minWidth: 0 }}>
      <span style={{ fontSize: 12.5, fontWeight: 600, color: 'var(--text-2)', paddingLeft: 2 }}>
        {label}
      </span>
      <input
        ref={inputRef}
        type={type}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        style={{
          height: 42,
          padding: '0 14px',
          borderRadius: 'var(--r-sm)',
          background: 'var(--field)',
          border: '1.5px solid var(--border)',
          color: 'var(--text)',
          fontSize: 14.5,
          outline: 'none',
          minWidth: 140,
        }}
      />
    </label>
  );
}

/* ============================================================================
 * Services — OAuth relying-party (client) management
 *
 * Replaces the placeholder with the full client registry (add-client-consent-
 * management): a table of registered clients (name, type, redirect URIs, scopes,
 * per-client consent count), a "Register service" create form that surfaces the
 * one-time client secret for confidential clients, an inline edit form (PATCH),
 * delete (confirm), and a per-client "revoke all grants" action. All calls go to
 * the session+role-gated `/api/v1/admin/services*` routes (design D1); mutations
 * are audited server-side. The console gate is enforced by the shell above.
 * ==========================================================================*/

/** Split a free-text scope/grant list (spaces, commas, newlines) into tokens. */
function splitTokens(raw: string): string[] {
  return raw
    .split(/[\s,]+/)
    .map((s) => s.trim())
    .filter(Boolean);
}

/** Split a redirect-URI textarea (one per line) into trimmed, non-empty lines. */
function splitLines(raw: string): string[] {
  return raw
    .split(/\r?\n/)
    .map((s) => s.trim())
    .filter(Boolean);
}

/**
 * Client-side mirror of the backend's `validRedirectURI` (adminapi/clients.go):
 * absolute http(s), has a host, no fragment. The server remains authoritative;
 * this only avoids a round-trip for an obviously bad URI.
 */
function isValidRedirectUri(raw: string): boolean {
  try {
    const u = new URL(raw.trim());
    if (u.protocol !== 'http:' && u.protocol !== 'https:') return false;
    if (!u.host) return false;
    if (u.hash) return false;
    return true;
  } catch {
    return false;
  }
}

/** A type badge mirroring the StatusBadge styling (accent = confidential). */
function ClientTypeBadge({ t, type }: { t: Dict; type: ClientType }): JSX.Element {
  const conf = type === 'confidential';
  return (
    <Badge
      color={conf ? 'var(--accent)' : 'var(--text-2)'}
      bg={conf ? 'var(--accent-soft)' : 'var(--surface-2)'}
    >
      {conf ? t.adm_services_type_confidential : t.adm_services_type_public}
    </Badge>
  );
}

/** A small wrapped list of pill chips for redirect URIs / scopes in the table. */
function Chips({ items, empty }: { items: string[]; empty: string }): JSX.Element {
  if (items.length === 0) {
    return <span style={{ fontSize: 13, color: 'var(--text-3)' }}>{empty}</span>;
  }
  return (
    <div className="row" style={{ gap: 6, flexWrap: 'wrap' }}>
      {items.map((s) => (
        <span
          key={s}
          style={{
            padding: '3px 9px',
            borderRadius: 'var(--r-pill)',
            background: 'var(--surface-2)',
            border: '1px solid var(--border)',
            fontSize: 12.5,
            color: 'var(--text-2)',
            maxWidth: 240,
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
          }}
        >
          {s}
        </span>
      ))}
    </div>
  );
}

/** Localised "N consents" label (RU/EN plural-aware, kept simple). */
function consentLabel(t: Dict, n: number): string {
  return n === 1 ? t.adm_services_consents_one : t.adm_services_consents_many;
}

export function AdminServicesScreen(): JSX.Element {
  const { t } = useApp();

  const [clients, setClients] = useState<Client[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // Which inline panel is open: the create form, or an edit form for a client id.
  const [creating, setCreating] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  // The one-time secret panel (confidential create) — id + secret, shown once.
  const [newSecret, setNewSecret] = useState<{ id: string; secret: string } | null>(null);

  const load = useCallback((signal?: AbortSignal) => {
    setError(null);
    return adminServicesApi
      .list(signal)
      .then((list) => {
        setClients(list);
        // Lazily hydrate each client's consent count (best-effort; failures are
        // swallowed so one bad client does not blank the whole table).
        list.forEach((c) => {
          adminServicesApi
            .consentCount(c.id, signal)
            .then((count) =>
              setClients((prev) =>
                prev ? prev.map((x) => (x.id === c.id ? { ...x, consentCount: count } : x)) : prev,
              ),
            )
            .catch(() => {
              /* best-effort per design D3 — leave the count undefined */
            });
        });
      })
      .catch((err: unknown) => {
        if (signal?.aborted) return;
        setError(err instanceof ApiError ? err.message : t.err_generic);
        setClients([]);
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  // Run a mutation behind an optional confirm, refresh, and surface a result.
  async function run(
    fn: () => Promise<unknown>,
    doneMsg: string,
    confirmMsg?: string,
  ): Promise<void> {
    if (busy) return;
    if (confirmMsg && !window.confirm(confirmMsg)) return;
    setError(null);
    setSuccess(null);
    setBusy(true);
    try {
      await fn();
      await load();
      setSuccess(doneMsg);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t.err_generic);
    } finally {
      setBusy(false);
    }
  }

  async function handleCreate(input: CreateClientInput): Promise<void> {
    if (busy) return;
    setError(null);
    setSuccess(null);
    setBusy(true);
    try {
      const created = await adminServicesApi.create(input);
      await load();
      setCreating(false);
      setSuccess(t.adm_services_created);
      if (created.secret) {
        setNewSecret({ id: created.client.id, secret: created.secret });
      }
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t.err_generic);
    } finally {
      setBusy(false);
    }
  }

  async function handleUpdate(id: string, input: UpdateClientInput): Promise<void> {
    await run(() => adminServicesApi.update(id, input), t.adm_services_updated);
    setEditingId(null);
  }

  return (
    <div className="col" style={{ gap: 22 }}>
      <div
        className="row"
        style={{
          justifyContent: 'space-between',
          alignItems: 'flex-end',
          gap: 14,
          flexWrap: 'wrap',
        }}
      >
        <div className="col" style={{ gap: 4 }}>
          <h1 style={{ fontFamily: 'var(--sans)', fontWeight: 400, fontSize: 34, lineHeight: 1 }}>
            {t.adm_services_title}
          </h1>
          <p style={{ fontSize: 14.5, color: 'var(--text-3)' }}>{t.adm_services_sub}</p>
        </div>
        <Button
          icon="plus"
          disabled={busy}
          onClick={() => {
            setEditingId(null);
            setSuccess(null);
            setError(null);
            setCreating((v) => !v);
          }}
        >
          {t.adm_services_register}
        </Button>
      </div>

      {error && <Notice tone="error">{error}</Notice>}
      {success && <Notice tone="success">{success}</Notice>}

      {newSecret && (
        <SecretPanel
          t={t}
          clientId={newSecret.id}
          secret={newSecret.secret}
          onDone={() => setNewSecret(null)}
        />
      )}

      {creating && (
        <ClientForm t={t} busy={busy} onSubmit={handleCreate} onCancel={() => setCreating(false)} />
      )}

      <Panel pad={0} style={{ overflow: 'hidden' }}>
        <div
          className="row adm-thead"
          style={{
            gap: 14,
            padding: '16px 24px',
            fontSize: 12.5,
            fontWeight: 700,
            color: 'var(--text-3)',
            textTransform: 'uppercase',
            letterSpacing: '.05em',
            borderBottom: '1px solid var(--border)',
          }}
        >
          <span style={{ flex: 1.4 }}>{t.adm_services_th_name}</span>
          <span style={{ flex: 0.9 }}>{t.adm_services_th_type}</span>
          <span style={{ flex: 2 }}>{t.adm_services_th_redirects}</span>
          <span style={{ flex: 1.6 }}>{t.adm_services_th_scopes}</span>
          <span style={{ flex: 0.9, textAlign: 'center' }}>{t.adm_services_th_consents}</span>
          <span style={{ width: 40 }} />
        </div>

        {clients === null ? (
          <p style={{ fontSize: 14, color: 'var(--text-3)', padding: '20px 24px' }}>
            {t.adm_services_loading}
          </p>
        ) : clients.length === 0 ? (
          <p style={{ fontSize: 14.5, color: 'var(--text-3)', padding: '20px 24px' }}>
            {t.adm_services_empty}
          </p>
        ) : (
          clients.map((c, i) => (
            <ClientRow
              key={c.id}
              t={t}
              client={c}
              busy={busy}
              last={i === clients.length - 1}
              editing={editingId === c.id}
              onToggleEdit={() => {
                setCreating(false);
                setSuccess(null);
                setError(null);
                setEditingId((cur) => (cur === c.id ? null : c.id));
              }}
              onSubmitEdit={(input) => handleUpdate(c.id, input)}
              onDelete={() =>
                run(
                  () => adminServicesApi.delete(c.id),
                  t.adm_services_deleted,
                  t.adm_services_delete_confirm,
                )
              }
              onRevoke={() =>
                run(
                  () => adminServicesApi.revokeConsents(c.id),
                  t.adm_services_revoked,
                  t.adm_services_revoke_confirm,
                )
              }
            />
          ))
        )}
      </Panel>
    </div>
  );
}

/** One client row: the summary line plus (when expanded) the inline edit form. */
function ClientRow({
  t,
  client,
  busy,
  last,
  editing,
  onToggleEdit,
  onSubmitEdit,
  onDelete,
  onRevoke,
}: {
  t: Dict;
  client: Client;
  busy: boolean;
  last: boolean;
  editing: boolean;
  onToggleEdit: () => void;
  onSubmitEdit: (input: UpdateClientInput) => void;
  onDelete: () => void;
  onRevoke: () => void;
}): JSX.Element {
  return (
    <div style={{ borderBottom: last && !editing ? 'none' : '1px solid var(--border)' }}>
      <div className="row adm-row" style={{ gap: 14, padding: '14px 24px', alignItems: 'center' }}>
        <div className="col" style={{ flex: 1.4, gap: 2, minWidth: 0 }}>
          <span style={{ fontSize: 15, fontWeight: 600 }}>{client.name || client.id}</span>
          <span
            style={{
              fontSize: 12.5,
              color: 'var(--text-3)',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
          >
            {client.id}
          </span>
        </div>
        <div style={{ flex: 0.9 }}>
          <ClientTypeBadge t={t} type={client.type} />
        </div>
        <div style={{ flex: 2, minWidth: 0 }}>
          <Chips items={client.redirectUris} empty="—" />
        </div>
        <div style={{ flex: 1.6, minWidth: 0 }}>
          <Chips items={client.scopes} empty="—" />
        </div>
        <span
          style={{
            flex: 0.9,
            textAlign: 'center',
            fontSize: 15,
            fontWeight: 600,
            color: 'var(--text-2)',
          }}
          title={
            client.consentCount === undefined
              ? undefined
              : `${client.consentCount} ${consentLabel(t, client.consentCount)}`
          }
        >
          {client.consentCount === undefined ? '…' : client.consentCount}
        </span>
        <div className="row" style={{ width: 40, justifyContent: 'flex-end', gap: 4 }}>
          <button
            type="button"
            aria-label={t.adm_services_edit}
            title={t.adm_services_edit}
            disabled={busy}
            onClick={onToggleEdit}
            style={{
              width: 30,
              height: 30,
              borderRadius: 9,
              display: 'grid',
              placeItems: 'center',
              color: editing ? 'var(--accent)' : 'var(--text-3)',
              opacity: busy ? 0.5 : 1,
              cursor: busy ? 'not-allowed' : 'pointer',
            }}
          >
            <Icon name="pencil" size={16} />
          </button>
        </div>
      </div>

      {/* per-row actions */}
      <div className="row" style={{ gap: 10, padding: '0 24px 14px', flexWrap: 'wrap' }}>
        <Button
          size="sm"
          variant="glass"
          icon="shield"
          disabled={busy}
          onClick={onRevoke}
          title={t.adm_services_revoke}
        >
          {t.adm_services_revoke}
        </Button>
        <Button size="sm" variant="danger" icon="trash" disabled={busy} onClick={onDelete}>
          {t.adm_services_delete}
        </Button>
      </div>

      {editing && (
        <div style={{ padding: '0 24px 18px' }}>
          <ClientForm
            t={t}
            busy={busy}
            initial={client}
            onSubmit={(input) => onSubmitEdit(input)}
            onCancel={onToggleEdit}
          />
        </div>
      )}
    </div>
  );
}

/**
 * The shared create/edit form. When `initial` is supplied it edits that client
 * (PATCH — the client type is fixed and shown read-only, per design D2); otherwise
 * it registers a new client (the type is selectable). Emits the typed input on
 * submit after a light client-side validation that mirrors the server.
 */
function ClientForm({
  t,
  busy,
  initial,
  onSubmit,
  onCancel,
}: {
  t: Dict;
  busy: boolean;
  initial?: Client;
  onSubmit: (input: CreateClientInput) => void;
  onCancel: () => void;
}): JSX.Element {
  const editing = initial !== undefined;
  const [name, setName] = useState(initial?.name ?? '');
  const [type, setType] = useState<ClientType>(initial?.type ?? 'public');
  const [redirects, setRedirects] = useState((initial?.redirectUris ?? []).join('\n'));
  const [scopes, setScopes] = useState(
    (initial?.scopes ?? ['openid', 'profile', 'email']).join(' '),
  );
  const [grants, setGrants] = useState(
    (initial?.grantTypes ?? ['authorization_code', 'refresh_token']).join(' '),
  );
  const [responses, setResponses] = useState((initial?.responseTypes ?? ['code']).join(' '));
  const [formError, setFormError] = useState<string | null>(null);

  function submit(): void {
    const trimmedName = name.trim();
    if (!trimmedName) {
      setFormError(t.adm_services_err_name);
      return;
    }
    const redirectList = splitLines(redirects);
    if (redirectList.length === 0 || !redirectList.every(isValidRedirectUri)) {
      setFormError(t.adm_services_err_redirects);
      return;
    }
    setFormError(null);
    onSubmit({
      name: trimmedName,
      type,
      redirectUris: redirectList,
      scopes: splitTokens(scopes),
      grantTypes: splitTokens(grants),
      responseTypes: splitTokens(responses),
    });
  }

  return (
    <Panel pad={24} className="col" style={{ gap: 16, border: '1px solid var(--accent-line)' }}>
      <SectionTitleAdm
        title={editing ? t.adm_services_edit_title : t.adm_services_register}
        sub={
          type === 'confidential'
            ? t.adm_services_type_help_confidential
            : t.adm_services_type_help_public
        }
      />

      {formError && <Notice tone="error">{formError}</Notice>}

      <div
        style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}
        className="adm-grid"
      >
        <Field
          label={t.adm_services_field_name}
          value={name}
          onChange={setName}
          icon="layers"
          autoFocus
        />
        <div className="col" style={{ gap: 7 }}>
          <span style={{ fontSize: 13.5, fontWeight: 600, color: 'var(--text-2)', paddingLeft: 2 }}>
            {t.adm_services_field_type}
          </span>
          {editing ? (
            // Type is fixed on edit (changing it would rotate auth method/secret).
            <div style={{ paddingTop: 8 }}>
              <ClientTypeBadge t={t} type={type} />
            </div>
          ) : (
            <Segmented<ClientType>
              value={type}
              onChange={setType}
              ariaLabel={t.adm_services_field_type}
              options={[
                { value: 'public', label: t.adm_services_type_public },
                { value: 'confidential', label: t.adm_services_type_confidential },
              ]}
            />
          )}
        </div>
      </div>

      <TextAreaField
        label={t.adm_services_field_redirects}
        value={redirects}
        onChange={setRedirects}
        placeholder={t.adm_services_field_redirects_ph}
        hint={t.adm_services_field_redirects_hint}
        rows={3}
      />

      <div
        style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr', gap: 16 }}
        className="adm-grid"
      >
        <Field
          label={t.adm_services_field_scopes}
          value={scopes}
          onChange={setScopes}
          hint={t.adm_services_field_scopes_hint}
        />
        <Field label={t.adm_services_field_grants} value={grants} onChange={setGrants} />
        <Field label={t.adm_services_field_responses} value={responses} onChange={setResponses} />
      </div>

      <div className="row" style={{ gap: 12, justifyContent: 'flex-end' }}>
        <Button variant="glass" disabled={busy} onClick={onCancel}>
          {editing ? t.adm_services_edit_cancel : t.adm_services_create_cancel}
        </Button>
        <Button icon="check" disabled={busy} onClick={submit}>
          {editing ? t.adm_services_edit_submit : t.adm_services_create_submit}
        </Button>
      </div>
    </Panel>
  );
}

/** A labelled glass <textarea> (Field is single-line; redirect URIs need lines). */
function TextAreaField({
  label,
  value,
  onChange,
  placeholder,
  hint,
  rows = 3,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  hint?: string;
  rows?: number;
}): JSX.Element {
  const [focus, setFocus] = useState(false);
  return (
    <label className="col" style={{ gap: 7, minWidth: 0 }}>
      <span style={{ fontSize: 13.5, fontWeight: 600, color: 'var(--text-2)', paddingLeft: 2 }}>
        {label}
      </span>
      <textarea
        value={value}
        rows={rows}
        placeholder={placeholder}
        onChange={(e) => onChange(e.target.value)}
        onFocus={() => setFocus(true)}
        onBlur={() => setFocus(false)}
        style={{
          padding: '12px 16px',
          borderRadius: 'var(--r-sm)',
          background: 'var(--field)',
          border: `1.5px solid ${focus ? 'var(--accent-line)' : 'var(--border)'}`,
          boxShadow: focus
            ? '0 0 0 4px var(--accent-soft)'
            : 'none',
          color: 'var(--text)',
          fontSize: 15,
          fontFamily: 'var(--mono, monospace)',
          outline: 'none',
          resize: 'vertical',
          transition: 'all .3s var(--ease)',
        }}
      />
      {hint && (
        <span style={{ fontSize: 12.5, color: 'var(--text-3)', paddingLeft: 2 }}>{hint}</span>
      )}
    </label>
  );
}

/**
 * The one-time client-secret panel (design D4). Shows the secret with a copy
 * button; it is held only in component state and discarded when the operator
 * dismisses it (it is never re-served by the API).
 */
function SecretPanel({
  t,
  clientId,
  secret,
  onDone,
}: {
  t: Dict;
  clientId: string;
  secret: string;
  onDone: () => void;
}): JSX.Element {
  const [copied, setCopied] = useState(false);

  async function copy(): Promise<void> {
    try {
      await navigator.clipboard?.writeText(secret);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      /* clipboard blocked — the secret is visible for manual copy */
    }
  }

  return (
    <Panel pad={24} className="col" style={{ gap: 14, border: '1px solid var(--accent-line)' }}>
      <div className="row" style={{ gap: 12, alignItems: 'flex-start' }}>
        <div
          style={{
            width: 40,
            height: 40,
            borderRadius: 12,
            flexShrink: 0,
            display: 'grid',
            placeItems: 'center',
            background: 'var(--accent-soft)',
            color: 'var(--accent)',
            border: '1px solid var(--accent-line)',
          }}
        >
          <Icon name="key" size={20} />
        </div>
        <div className="col" style={{ gap: 3 }}>
          <h3 style={{ fontSize: 18, fontWeight: 600 }}>{t.adm_services_secret_title}</h3>
          <p style={{ fontSize: 13.5, color: 'var(--text-3)', lineHeight: 1.5 }}>
            {t.adm_services_secret_body}
          </p>
          <span style={{ fontSize: 12.5, color: 'var(--text-3)' }}>{clientId}</span>
        </div>
      </div>

      <div
        className="row"
        style={{
          gap: 10,
          padding: '12px 16px',
          borderRadius: 'var(--r-sm)',
          background: 'var(--field)',
          border: '1px solid var(--border)',
        }}
      >
        <code
          style={{
            flex: 1,
            fontSize: 14,
            fontFamily: 'var(--mono, monospace)',
            color: 'var(--text)',
            wordBreak: 'break-all',
          }}
        >
          {secret}
        </code>
        <Button
          size="sm"
          variant="glass"
          icon={copied ? 'check' : 'doc'}
          onClick={() => void copy()}
        >
          {copied ? t.adm_services_secret_copied : t.adm_services_secret_copy}
        </Button>
      </div>

      <div className="row" style={{ justifyContent: 'flex-end' }}>
        <Button icon="check" onClick={onDone}>
          {t.adm_services_secret_done}
        </Button>
      </div>
    </Panel>
  );
}

/* ============================================================================
 * Settings — minimal read-only system info + appearance toggles
 * ==========================================================================*/

export function AdminSettingsScreen(): JSX.Element {
  const { t, lang, theme, setLang, setTheme } = useApp();
  return (
    <div className="col" style={{ gap: 22 }}>
      <div className="col" style={{ gap: 4 }}>
        <h1 style={{ fontFamily: 'var(--sans)', fontWeight: 400, fontSize: 34, lineHeight: 1 }}>
          {t.adm_settings_title}
        </h1>
        <p style={{ fontSize: 14.5, color: 'var(--text-3)' }}>{t.adm_settings_sub}</p>
      </div>

      <div
        style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 18 }}
        className="adm-grid"
      >
        <Panel pad={26} className="col" style={{ gap: 14 }}>
          <SectionTitleAdm title={t.adm_settings_system} />
          <ListRow icon="grid" title="cotton-id" sub={t.adm_brand} />
          <ListRow icon="bolt" title={t.adm_settings_env} sub={import.meta.env.MODE} />
        </Panel>

        <Panel pad={26} className="col" style={{ gap: 18 }}>
          <SectionTitleAdm title={t.adm_settings_appearance} />
          <div className="row" style={{ justifyContent: 'space-between', gap: 12 }}>
            <span style={{ fontSize: 15.5, fontWeight: 600 }}>{t.acc_theme}</span>
            <Segmented<'dark' | 'light'>
              size="sm"
              value={theme}
              onChange={setTheme}
              ariaLabel={t.acc_theme}
              options={[
                { value: 'light', icon: 'sun' },
                { value: 'dark', icon: 'moon' },
              ]}
            />
          </div>
          <div className="row" style={{ justifyContent: 'space-between', gap: 12 }}>
            <span style={{ fontSize: 15.5, fontWeight: 600 }}>{t.acc_lang}</span>
            <Segmented<Lang>
              size="sm"
              value={lang}
              onChange={setLang}
              ariaLabel={t.acc_lang}
              options={[
                { value: 'ru', label: 'RU' },
                { value: 'en', label: 'EN' },
              ]}
            />
          </div>
        </Panel>
      </div>
    </div>
  );
}
