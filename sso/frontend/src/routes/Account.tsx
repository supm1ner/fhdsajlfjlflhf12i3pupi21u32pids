/* ============================================================================
 * Account.tsx — `/account` new design (6-tab sidebar layout)
 * ==========================================================================*/

import { useCallback, useEffect, useRef, useState } from 'react';
import { create as webauthnCreate } from '@github/webauthn-json';
import { useNavigate } from 'react-router-dom';
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
import { Toggle } from '../components/Toggle';
import { useApp, type Theme } from '../i18n/context';
import type { Dict, Lang } from '../i18n/dictionary';
import {
  ApiError, accountApi, passkeyApi, passkeysSupported,
  type Account, type AccountConnection, type AccountPreferences,
  type AccountSession, type ImageKind, type Passkey,
} from '../lib/api';
import { isPasskeyCancellation } from '../lib/passkey';

const IMAGE_TYPES = ['image/png', 'image/jpeg', 'image/webp'];
const IMAGE_MAX_BYTES: Record<ImageKind, number> = { avatar: 512 * 1024, banner: 1024 * 1024 };

function fmtMax(kind: ImageKind): string { return kind === 'avatar' ? '512 KB' : '1 MB'; }
function fmtDateTime(iso: string | undefined, lang: string): string | null {
  if (!iso) return null;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return null;
  return d.toLocaleString(lang === 'ru' ? 'ru-RU' : 'en-US', { year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
}
function fmtDate(iso: string | undefined, lang: string): string | null {
  if (!iso) return null;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return null;
  return d.toLocaleDateString(lang === 'ru' ? 'ru-RU' : 'en-US', { year: 'numeric', month: 'long' });
}
function deviceIcon(userAgent: string): IconName { return /Mobi|Android|iPhone|iPad/i.test(userAgent) ? 'smartphone' : 'monitor'; }
function deviceLabel(userAgent: string): string {
  const ua = userAgent || '';
  const os = /Windows/i.test(ua) ? 'Windows' : /Mac OS X|Macintosh/i.test(ua) ? 'macOS' : /iPhone|iPad|iOS/i.test(ua) ? 'iOS' : /Android/i.test(ua) ? 'Android' : /Linux/i.test(ua) ? 'Linux' : null;
  const browser = /Edg\//i.test(ua) ? 'Edge' : /Chrome\//i.test(ua) ? 'Chrome' : /Safari\//i.test(ua) ? 'Safari' : /Firefox\//i.test(ua) ? 'Firefox' : null;
  const parts = [browser, os].filter(Boolean);
  return parts.length ? parts.join(' · ') : ua.slice(0, 40) || 'Unknown device';
}

type TabId = 'overview' | 'profile' | 'security' | 'devices' | 'apps' | 'privacy';
const NAV: ReadonlyArray<{ key: TabId; icon: IconName; ru: string; en: string }> = [
  { key: 'overview', icon: 'grid', ru: 'Обзор', en: 'Overview' },
  { key: 'profile', icon: 'user', ru: 'Личные данные', en: 'Personal info' },
  { key: 'security', icon: 'shield-check', ru: 'Безопасность', en: 'Security' },
  { key: 'devices', icon: 'monitor', ru: 'Устройства', en: 'Devices' },
  { key: 'apps', icon: 'apps', ru: 'Приложения', en: 'Apps' },
  { key: 'privacy', icon: 'lock', ru: 'Приватность', en: 'Privacy' },
];

function PageTitle({ title, desc }: { title: string; desc?: string }): JSX.Element {
  return <div style={{ marginBottom: 24 }}><h1 style={{ fontSize: 26, letterSpacing: '-0.02em' }}>{title}</h1>{desc && <p className="muted" style={{ fontSize: 15, marginTop: 7 }}>{desc}</p>}</div>;
}

function SettingRow({ icon, title, sub, right, onClick, last }: { icon?: IconName; title: string; sub?: string; right?: React.ReactNode; onClick?: () => void; last?: boolean }): JSX.Element {
  return (
    <div onClick={onClick} className="row gap-3" style={{ padding: '16px 4px', borderBottom: last ? 'none' : '1px solid var(--border)', cursor: onClick ? 'pointer' : 'default' }}>
      {icon && <div style={{ width: 38, height: 38, flex: '0 0 auto', borderRadius: 10, background: 'var(--surface-2)', border: '1px solid var(--border)', display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--text-2)' }}><Icon name={icon} size={18} /></div>}
      <div style={{ flex: 1, minWidth: 0 }}><div style={{ fontSize: 14.5, fontWeight: 540 }}>{title}</div>{sub && <div className="faint" style={{ fontSize: 13, marginTop: 2 }}>{sub}</div>}</div>
      {right}
    </div>
  );
}

function Divider(): JSX.Element { return <div style={{ height: 1, background: 'var(--border)', margin: '4px 0' }} />; }

function applyServerPreferences(prefs: AccountPreferences, setTheme: (t: Theme) => void, setLang: (l: Lang) => void): void {
  const resolvedTheme: Theme = prefs.theme === 'system' ? (typeof window !== 'undefined' && window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light') : prefs.theme;
  setTheme(resolvedTheme); setLang(prefs.lang);
}

export function Account(): JSX.Element {
  const { t, lang, theme, setLang, setTheme } = useApp();
  const navigate = useNavigate();
  const [account, setAccount] = useState<Account | null>(null);
  const [loaded, setLoaded] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [tab, setTab] = useState<TabId>('overview');

  useEffect(() => {
    const controller = new AbortController();
    accountApi.get(controller.signal)
      .then((acc) => { setAccount(acc); setLoaded(true); applyServerPreferences(acc.preferences, setTheme, setLang); })
      .catch((err: unknown) => {
        if (controller.signal.aborted) return;
        if (err instanceof ApiError && err.status === 401) { navigate('/login'); return; }
        setLoadError(err instanceof ApiError ? err.message : t.err_generic); setLoaded(true);
      });
    return () => controller.abort();
  }, [navigate, setTheme, setLang, t.err_generic]);

  if (!loaded) return <div style={{ minHeight: '100vh', display: 'grid', placeItems: 'center' }}><p style={{ fontSize: 15, color: 'var(--text-3)' }}>{t.acc_loading}</p></div>;
  if (loadError || !account) return <div style={{ minHeight: '100vh', display: 'grid', placeItems: 'center', padding: 24 }}><div style={{ maxWidth: 420, width: '100%' }}><Notice tone="error">{loadError ?? t.err_generic}</Notice></div></div>;

  const Views: Record<TabId, React.FC<{ t: Dict; lang: string; account: Account; onChange: (a: Account) => void }>> = {
    overview: OverviewView, profile: ProfileView, security: SecurityView,
    devices: DevicesView, apps: AppsView, privacy: PrivacyView,
  };
  const View = Views[tab];

  return (
    <div className="screen-min" style={{ display: 'flex', flexDirection: 'column' }}>
      <header style={{ position: 'sticky', top: 0, zIndex: 30, display: 'flex', alignItems: 'center', gap: 14, padding: '14px clamp(16px,4vw,32px)', borderBottom: '1px solid var(--border)', background: 'color-mix(in srgb, var(--bg) 85%, transparent)', backdropFilter: 'blur(12px)' }}>
        <Logo size={19} accentMark />
        <span style={{ fontSize: 13, padding: '3px 9px', borderRadius: 7, background: 'var(--surface-2)', border: '1px solid var(--border)', color: 'var(--text-2)', fontWeight: 540 }}>ID</span>
        <span className="spacer" />
        <Button variant="soft" size="sm" icon="settings" onClick={() => navigate('/admin')}>{t('Админка', 'Admin')}</Button>
        <LangSwitch /><ThemeSwitch />
        <div className="vdivider" style={{ height: 26, margin: '0 2px' }} />
        <button className="btn btn-quiet btn-sm" onClick={() => navigate('/')} style={{ paddingLeft: 6 }}><Avatar name={account.displayName || account.username} size={28} /></button>
      </header>
      <div style={{ flex: 1, display: 'flex', maxWidth: 1180, width: '100%', margin: '0 auto' }}>
        <aside style={{ width: 244, flex: '0 0 auto', padding: '26px 14px', borderRight: '1px solid var(--border)', position: 'sticky', top: 57, alignSelf: 'flex-start', height: 'calc(100vh - 57px)' }}>
          <nav className="col gap-1">
            {NAV.map((n) => (
              <button key={n.key} onClick={() => setTab(n.key)}
                className="row gap-3" style={{ padding: '10px 12px', borderRadius: 10, cursor: 'pointer', border: 'none', textAlign: 'left', width: '100%', background: tab === n.key ? 'var(--surface-2)' : 'transparent', color: tab === n.key ? 'var(--text)' : 'var(--text-2)', fontWeight: tab === n.key ? 600 : 500, fontSize: 14.5, transition: 'all .14s' }}>
                <Icon name={n.icon} size={18} style={{ color: tab === n.key ? 'var(--accent)' : 'inherit' }} /> {t(n.ru, n.en)}
              </button>
            ))}
          </nav>
        </aside>
        <main style={{ flex: 1, minWidth: 0, padding: 'clamp(24px,4vw,44px)', maxWidth: 820 }}>
          <View t={t} lang={lang} account={account} onChange={setAccount} />
        </main>
      </div>
    </div>
  );
}

/* ===================== Overview ===================== */

function QuickCard({ ic, h, s, onClick }: { ic: IconName; h: string; s: string; onClick: () => void }): JSX.Element {
  return (
    <button className="card" onClick={onClick} style={{ padding: 18, textAlign: 'left', cursor: 'pointer', display: 'flex', flexDirection: 'column', gap: 12, background: 'var(--surface)' }}>
      <div className="row"><div style={{ color: 'var(--accent)' }}><Icon name={ic} size={22} /></div><span className="spacer" /><Icon name="chevron-right" size={17} style={{ color: 'var(--text-3)' }} /></div>
      <div><div style={{ fontWeight: 600, fontSize: 15 }}>{h}</div><div className="faint" style={{ fontSize: 13, marginTop: 2 }}>{s}</div></div>
    </button>
  );
}

function OverviewView({ t, lang, account, onChange }: { t: Dict; lang: string; account: Account; onChange: (a: Account) => void }): JSX.Element {
  const navigate = useNavigate();
  const cards = [
    { ic: 'fingerprint' as IconName, h: t('Passkey', 'Passkey'), s: t('{n} активных ключей', '{n} active keys').replace('{n}', String(account.counts.passkeys)), dest: 'security' },
    { ic: 'monitor' as IconName, h: t('Устройства', 'Devices'), s: t('{n} сессий', '{n} sessions').replace('{n}', String(account.counts.sessions)), dest: 'devices' },
    { ic: 'apps' as IconName, h: t('Приложения', 'Apps'), s: t('{n} подключено', '{n} connected').replace('{n}', String(account.counts.connections)), dest: 'apps' },
    { ic: 'shield-check' as IconName, h: '2FA', s: t('Безопасность', 'Security'), dest: 'security' },
  ];
  return (
    <div className="fade-up">
      <div className="card" style={{ padding: 28, marginBottom: 18, display: 'flex', alignItems: 'center', gap: 22, flexWrap: 'wrap' }}>
        <Avatar name={account.displayName || account.username} size={72} />
        <div style={{ flex: 1, minWidth: 180 }}>
          <h2 style={{ fontSize: 22 }}>{account.displayName || account.username}</h2>
          <div className="mono muted" style={{ fontSize: 13.5, marginTop: 4 }}>{account.email}</div>
          <div className="row gap-2" style={{ marginTop: 12 }}>
            <Badge tone="good" dot>{t('Подтверждён', 'Verified')}</Badge>
            {account.counts.passkeys > 0 && <Badge icon="fingerprint">Passkey</Badge>}
          </div>
        </div>
        <Button variant="soft" icon="edit" onClick={() => onChange({ ...account })}>{t('Редактировать', 'Edit')}</Button>
      </div>
      <div className="card card-pad" style={{ marginBottom: 18 }}>
        <div style={{ display: 'flex', gap: 16, alignItems: 'center', flexWrap: 'wrap' }}>
          <div style={{ width: 84, height: 84, flex: '0 0 auto', position: 'relative', borderRadius: '50%', background: 'var(--good-tint)', display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center' }}>
            <span style={{ fontSize: 22, fontWeight: 600, color: 'var(--good)' }}>85</span>
            <span className="faint" style={{ fontSize: 10 }}>{t('из 100', 'of 100')}</span>
          </div>
          <div style={{ flex: 1, minWidth: 200 }}>
            <h3 style={{ fontSize: 17 }}>{t('Безопасность аккаунта', 'Account security')}</h3>
            <p className="muted" style={{ fontSize: 14, marginTop: 5, lineHeight: 1.5 }}>{t('Хорошая защита.', 'Good protection.')}</p>
          </div>
          <Button variant="accent" onClick={() => {}}>{t('Улучшить', 'Improve')}</Button>
        </div>
      </div>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px,1fr))', gap: 14 }}>
        {cards.map((c) => <QuickCard key={c.h} ic={c.ic} h={c.h} s={c.s} onClick={() => {}} />)}
      </div>
    </div>
  );
}

/* ===================== Profile ===================== */

function ProfileView({ t, lang, account, onChange }: { t: Dict; lang: string; account: Account; onChange: (a: Account) => void }): JSX.Element {
  const [name, setName] = useState(account.displayName);
  const [about, setAbout] = useState(account.about);
  const [loc, setLoc] = useState(account.location);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [bannerPreview, setBannerPreview] = useState<string | null>(null);
  const [avatarPreview, setAvatarPreview] = useState<string | null>(null);
  const [imgError, setImgError] = useState<string | null>(null);
  const [imgBusy, setImgBusy] = useState(false);
  const bannerInput = useRef<HTMLInputElement>(null);
  const avatarInput = useRef<HTMLInputElement>(null);

  const dirty = name !== account.displayName || about !== account.about || loc !== account.location;

  function reset(): void { setName(account.displayName); setAbout(account.about); setLoc(account.location); setError(null); setSuccess(null); }

  async function save(): Promise<void> {
    if (busy || !dirty) return; setError(null); setSuccess(null); setBusy(true);
    try {
      const updated = await accountApi.updateProfile({ displayName: name.trim(), about, location: loc });
      onChange({ ...updated, counts: account.counts }); setName(updated.displayName); setAbout(updated.about); setLoc(updated.location); setSuccess(t.acc_saved);
    } catch (err) { setError(err instanceof ApiError ? err.message : t.err_generic); } finally { setBusy(false); }
  }

  async function onFile(kind: ImageKind, file: File | undefined): Promise<void> {
    if (!file || imgBusy) return; setImgError(null);
    if (!IMAGE_TYPES.includes(file.type)) { setImgError(t.acc_img_type_err); return; }
    if (file.size > IMAGE_MAX_BYTES[kind]) { setImgError(t.acc_img_size_err.replace('{max}', fmtMax(kind))); return; }
    const url = URL.createObjectURL(file); if (kind === 'avatar') setAvatarPreview(url); else setBannerPreview(url); setImgBusy(true);
    try {
      const { url: served } = await accountApi.uploadImage(kind, file);
      onChange({ ...account, ...(kind === 'avatar' ? { avatarUrl: served } : { bannerUrl: served }) });
    } catch (err) { setImgError(err instanceof ApiError ? err.message : t.err_generic); if (kind === 'avatar') setAvatarPreview(null); else setBannerPreview(null); } finally { setImgBusy(false); }
  }

  return (
    <div className="fade-up">
      <PageTitle title={t('Личные данные', 'Personal info')} desc={t('Эти данные видят сервисы cotton, которым вы дали доступ.', 'This info is shared with cotton services you authorize.')} />
      <div className="card" style={{ padding: 28, marginBottom: 18, display: 'flex', alignItems: 'center', gap: 20 }}>
        <input ref={avatarInput} type="file" accept={IMAGE_TYPES.join(',')} hidden aria-label={t.acc_change_avatar} onChange={(e) => void onFile('avatar', e.target.files?.[0])} />
        <input ref={bannerInput} type="file" accept={IMAGE_TYPES.join(',')} hidden aria-label={t.acc_change_banner} onChange={(e) => void onFile('banner', e.target.files?.[0])} />
        <div style={{ cursor: 'pointer' }} onClick={() => avatarInput.current?.click()}>
          <Avatar name={account.displayName || account.username} src={avatarPreview ?? account.avatarUrl} size={64} />
        </div>
        <div style={{ flex: 1 }}><div style={{ fontWeight: 600 }}>{t('Фото профиля', 'Profile photo')}</div><div className="faint" style={{ fontSize: 13, marginTop: 3 }}>{'PNG, JPG · max 5 MB'}</div></div>
        <Button variant="soft" size="sm" onClick={() => avatarInput.current?.click()}>{t('Загрузить', 'Upload')}</Button>
      </div>
      <div className="card" style={{ padding: '8px 24px', marginBottom: 18 }}>
        {error && <Notice tone="error">{error}</Notice>}
        {success && <Notice tone="success">{success}</Notice>}
        {imgError && <Notice tone="error">{imgError}</Notice>}
        <SettingRow icon="user" title={t.f_name} sub={name} right={<Button variant="quiet" size="sm" onClick={() => setName('')}>{t('Изменить', 'Edit')}</Button>} />
        <SettingRow icon="user" title={t.f_username} sub={`@${account.username}`} />
        <SettingRow icon="mail" title="Email" sub={account.email} right={<Badge tone="good">{t('Основной', 'Primary')}</Badge>} last />
      </div>
      <div className="row" style={{ gap: 12 }}>
        <Button icon="check" onClick={save} disabled={busy || !dirty}>{t.acc_save}</Button>
        <Button variant="ghost" onClick={reset} disabled={busy || !dirty}>{t.acc_cancel}</Button>
      </div>
    </div>
  );
}

/* ===================== Security ===================== */

function SecurityView({ t, lang, account, onChange }: { t: Dict; lang: string; account: Account; onChange: (a: Account) => void }): JSX.Element {
  const supported = passkeysSupported();
  const [passkeys, setPasskeys] = useState<Passkey[] | null>(null);
  const [pwdBusy, setPwdBusy] = useState(false);
  const [pwdError, setPwdError] = useState<string | null>(null);
  const [pwdSuccess, setPwdSuccess] = useState<string | null>(null);
  const [pkBusy, setPkBusy] = useState(false);
  const [pkError, setPkError] = useState<string | null>(null);
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [confirm, setConfirm] = useState('');
  const [tfa, setTfa] = useState(true);

  useEffect(() => {
    const controller = new AbortController();
    passkeyApi.list(controller.signal).then(res => setPasskeys(res.passkeys ?? [])).catch(() => { if (!controller.signal.aborted) setPasskeys([]); });
    return () => controller.abort();
  }, []);

  async function changePassword(): Promise<void> {
    if (pwdBusy) return; setPwdError(null); setPwdSuccess(null);
    if (next !== confirm) { setPwdError(t.acc_pw_mismatch); return; }
    setPwdBusy(true);
    try { await accountApi.changePassword(current, next); setCurrent(''); setNext(''); setConfirm(''); setPwdSuccess(t.acc_pw_changed); }
    catch (err) { setPwdError(err instanceof ApiError ? err.status === 401 || err.status === 403 ? t.acc_pw_wrong : err.message : t.err_generic); }
    finally { setPwdBusy(false); }
  }

  async function addPasskey(): Promise<void> {
    if (pkBusy || !supported) return; setPkError(null);
    const name = window.prompt(t.pk_name_prompt, t.pk_name_default); if (name === null) return; setPkBusy(true);
    try { const { publicKey } = await passkeyApi.registerBegin(); const credential = await webauthnCreate({ publicKey }); await passkeyApi.registerFinish(credential, name.trim() || t.pk_name_default); const res = await passkeyApi.list(); setPasskeys(res.passkeys ?? []); onChange({ ...account, counts: { ...account.counts, passkeys: (res.passkeys ?? []).length } }); }
    catch (err) { if (isPasskeyCancellation(err)) { setPkBusy(false); return; } setPkError(err instanceof ApiError ? err.message : t.pk_err_register); } finally { setPkBusy(false); }
  }

  async function deletePasskey(id: string): Promise<void> {
    if (pkBusy) return; if (!window.confirm(t.pk_delete_confirm)) return; setPkError(null); setPkBusy(true);
    try { await passkeyApi.delete(id); const res = await passkeyApi.list(); setPasskeys(res.passkeys ?? []); onChange({ ...account, counts: { ...account.counts, passkeys: (res.passkeys ?? []).length } }); }
    catch (err) { setPkError(err instanceof ApiError ? err.message : t.pk_err_delete); } finally { setPkBusy(false); }
  }

  return (
    <div className="fade-up">
      <PageTitle title={t('Безопасность', 'Security')} desc={t('Управляйте входом и защитой cotton ID.', 'Manage how you sign in and protect cotton ID.')} />
      <div className="col gap-4">
        <div className="card card-pad">
          <div className="row" style={{ marginBottom: 8 }}>
            <div><h3 style={{ fontSize: 17 }}>{t('Ключи доступа (Passkey)', 'Passkeys')}</h3><p className="faint" style={{ fontSize: 13.5, marginTop: 4 }}>{t('Вход без пароля по биометрии', 'Passwordless sign-in with biometrics')}</p></div>
            <span className="spacer" />
            {supported && <Button variant="soft" size="sm" icon="plus" onClick={addPasskey} disabled={pkBusy}>{t('Добавить', 'Add')}</Button>}
          </div>
          {pkError && <Notice tone="error">{pkError}</Notice>}
          {(passkeys ?? []).map((p, i) => <SettingRow key={p.id} icon="smartphone" title={p.name || t.pk_name_default} sub={`${t('Добавлен', 'Added')} ${fmtDate(p.createdAt, lang) ?? ''}`} last={i === (passkeys ?? []).length - 1} right={<button className="btn btn-quiet btn-icon btn-sm" onClick={() => deletePasskey(p.id)}><Icon name="trash" size={16} /></button>} />)}
          {(passkeys ?? []).length === 0 && <p className="faint" style={{ fontSize: 14, padding: '12px 4px' }}>{t('Нет passkey', 'No passkeys yet')}</p>}
        </div>
        <div className="card" style={{ padding: '8px 24px' }}>
          {pwdError && <Notice tone="error">{pwdError}</Notice>}
          {pwdSuccess && <Notice tone="success">{pwdSuccess}</Notice>}
          <SettingRow icon="lock" title={t('Пароль', 'Password')} sub={t('Изменить пароль', 'Change your password')} right={<Button variant="soft" size="sm" onClick={() => {}}>{t('Изменить', 'Change')}</Button>} />
          <div style={{ padding: '12px 4px' }}>
            <div className="col gap-3">
              <Field label={t.acc_pw_current} type="password" value={current} onChange={setCurrent} icon="lock" name="currentPassword" autoComplete="current-password" />
              <div className="row gap-3">
                <Field label={t.acc_pw_new} type="password" value={next} onChange={setNext} icon="lock" name="newPassword" autoComplete="new-password" />
                <Field label={t.acc_pw_confirm} type="password" value={confirm} onChange={setConfirm} icon="lock" autoComplete="new-password" />
              </div>
              <Button size="sm" icon="check" onClick={changePassword} disabled={pwdBusy || !current || !next || !confirm}>{t.acc_pw_save}</Button>
            </div>
          </div>
          <SettingRow icon="shield-check" title={t('Двухфакторная аутентификация', 'Two-factor authentication')} sub={tfa ? t('Приложение-аутентификатор', 'Authenticator app') : t('Выключена', 'Off')} last
            right={<div className="row gap-3">{tfa && <Badge tone="good" dot>{t('Вкл', 'On')}</Badge>}<Toggle on={tfa} onChange={setTfa} label="2FA" /></div>} />
        </div>
        <div className="card card-pad">
          <h3 style={{ fontSize: 16, marginBottom: 6 }}>{t('Недавняя активность', 'Recent activity')}</h3>
          <Button variant="quiet" size="sm" iconRight="arrow-right">{t('Весь журнал', 'Full log')}</Button>
        </div>
      </div>
    </div>
  );
}

/* ===================== Devices ===================== */

function DevicesView({ t, lang }: { t: Dict; lang: string; account: Account; onChange: (a: Account) => void }): JSX.Element {
  const [sessions, setSessions] = useState<AccountSession[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    accountApi.sessions(controller.signal).then(res => setSessions(res.sessions ?? [])).catch((err: unknown) => { if (!controller.signal.aborted) { setError(err instanceof ApiError ? err.message : ''); setSessions([]); } });
    return () => controller.abort();
  }, []);

  async function revoke(id: string): Promise<void> {
    if (busy) return; if (!window.confirm(t.acc_revoke_confirm)) return; setError(null); setBusy(true);
    try { await accountApi.revokeSession(id); const res = await accountApi.sessions(); setSessions(res.sessions ?? []); } catch (err) { setError(err instanceof ApiError ? err.message : t.err_generic); } finally { setBusy(false); }
  }

  async function revokeOthers(): Promise<void> {
    if (busy) return; if (!window.confirm(t.acc_revoke_others_confirm)) return; setError(null); setBusy(true);
    try { await accountApi.revokeOtherSessions(); const res = await accountApi.sessions(); setSessions(res.sessions ?? []); } catch (err) { setError(err instanceof ApiError ? err.message : t.err_generic); } finally { setBusy(false); }
  }

  const hasOthers = (sessions ?? []).some(s => !s.current);

  return (
    <div className="fade-up">
      <PageTitle title={t('Устройства и сессии', 'Devices & sessions')} desc={t('Места, где выполнен вход в cotton ID.', 'Where you\'re currently signed in to cotton ID.')} />
      {error && <Notice tone="error">{error}</Notice>}
      <div className="row" style={{ marginBottom: 14 }}>
        <Badge dot>{(sessions ?? []).length} {t('активных', 'active')}</Badge>
        <span className="spacer" />
        {hasOthers && <Button variant="danger" size="sm" icon="logout" onClick={revokeOthers} disabled={busy}>{t('Выйти везде', 'Sign out everywhere')}</Button>}
      </div>
      <div className="col gap-3">
        {(sessions ?? []).map(s => (
          <div className="card" key={s.id} style={{ padding: 18, display: 'flex', alignItems: 'center', gap: 16 }}>
            <div style={{ width: 46, height: 46, flex: '0 0 auto', borderRadius: 12, background: s.current ? 'var(--accent-tint)' : 'var(--surface-2)', color: s.current ? 'var(--accent)' : 'var(--text-2)', border: '1px solid var(--border)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}><Icon name={deviceIcon(s.userAgent)} size={22} /></div>
            <div style={{ flex: 1, minWidth: 0 }}>
              <div className="row gap-2"><span style={{ fontWeight: 600, fontSize: 15 }}>{deviceLabel(s.userAgent)}</span>{s.current && <Badge tone="good">{t('Это устройство', 'This device')}</Badge>}</div>
              <div className="faint" style={{ fontSize: 13, marginTop: 3 }}>{s.ip || '—'} · {fmtDateTime(s.lastSeenAt ?? s.createdAt, lang) ?? '—'}</div>
            </div>
            {!s.current && <Button variant="ghost" size="sm" onClick={() => revoke(s.id)} disabled={busy}>{t('Выйти', 'Sign out')}</Button>}
          </div>
        ))}
      </div>
    </div>
  );
}

/* ===================== Apps ===================== */

function AppsView({ t, lang }: { t: Dict; lang: string; account: Account; onChange: (a: Account) => void }): JSX.Element {
  const [connections, setConnections] = useState<AccountConnection[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    accountApi.connections(controller.signal).then(res => setConnections(res.connections ?? [])).catch((err: unknown) => { if (!controller.signal.aborted) { setError(err instanceof ApiError ? err.message : ''); setConnections([]); } });
    return () => controller.abort();
  }, []);

  async function revoke(client: string): Promise<void> {
    if (busy) return; if (!window.confirm(t.acc_disconnect_confirm)) return; setError(null); setSuccess(null); setBusy(true);
    try { await accountApi.revokeConnection(client); const res = await accountApi.connections(); setConnections(res.connections ?? []); setSuccess(t.acc_service_revoked); } catch (err) { setError(err instanceof ApiError ? err.message : t.err_generic); } finally { setBusy(false); }
  }

  const thirdParty = (connections ?? []).filter(c => !c.client.startsWith('cotton_'));
  const cottonServices = (connections ?? []).filter(c => c.client.startsWith('cotton_'));

  return (
    <div className="fade-up">
      <PageTitle title={t('Подключённые приложения', 'Connected apps')} desc={t('Сервисы с доступом к вашему cotton ID.', 'Services with access to your cotton ID.')} />
      {error && <Notice tone="error">{error}</Notice>}
      {success && <Notice tone="success">{success}</Notice>}

      {thirdParty.length > 0 && <div className="eyebrow" style={{ margin: '0 0 12px 2px' }}>{t('Сторонние приложения', 'Third-party apps')}</div>}
      <div className="col gap-3" style={{ marginBottom: 26 }}>
        {thirdParty.length === 0 && connections !== null && <p className="faint" style={{ fontSize: 14 }}>{t('Нет сторонних приложений', 'No third-party apps')}</p>}
        {thirdParty.map(c => (
          <div key={c.client} className="card" style={{ padding: 18, display: 'flex', alignItems: 'center', gap: 16 }}>
            <div style={{ width: 44, height: 44, flex: '0 0 auto', borderRadius: 12, background: 'var(--accent-tint)', color: 'var(--accent)', display: 'flex', alignItems: 'center', justifyContent: 'center', fontWeight: 700, fontSize: 18 }}>{c.clientName.charAt(0).toUpperCase()}</div>
            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{ fontWeight: 600, fontSize: 15 }}>{c.clientName || c.client}</div>
              <div className="faint" style={{ fontSize: 13, marginTop: 2 }}>{c.grantedScopes.join(', ')} · {t('подключено ', 'connected ')}{fmtDate(c.grantedAt, lang) ?? ''}</div>
            </div>
            <Button variant="danger" size="sm" onClick={() => revoke(c.client)} disabled={busy}>{t('Отозвать', 'Revoke')}</Button>
          </div>
        ))}
      </div>

      {cottonServices.length > 0 && <div className="eyebrow" style={{ margin: '0 0 12px 2px' }}>{t('Сервисы cotton', 'cotton services')}</div>}
      {cottonServices.length > 0 && <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px,1fr))', gap: 12 }}>
        {cottonServices.map(c => (
          <div key={c.client} className="card" style={{ padding: 16, display: 'flex', alignItems: 'center', gap: 12 }}>
            <div style={{ width: 40, height: 40, flex: '0 0 auto', borderRadius: 11, background: 'var(--surface-2)', border: '1px solid var(--border)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}><Icon name="grid" size={19} /></div>
            <div style={{ flex: 1, minWidth: 0 }}><div style={{ fontWeight: 600, fontSize: 14 }}>{c.clientName || c.client}</div><div className="faint" style={{ fontSize: 12.5 }}>{c.grantedScopes.join(', ')}</div></div>
            <Badge tone="good" dot />
          </div>
        ))}
      </div>}
    </div>
  );
}

/* ===================== Privacy ===================== */

function PrivacyView({ t, account, onChange }: { t: Dict; lang: string; account: Account; onChange: (a: Account) => void }): JSX.Element {
  const [hist, setHist] = useState(true);
  const [ads, setAds] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);

  return (
    <div className="fade-up">
      <PageTitle title={t('Приватность и данные', 'Privacy & data')} desc={t('Контроль над тем, как cotton использует ваши данные.', 'Control how cotton uses your data.')} />
      <div className="col gap-4">
        <div className="card" style={{ padding: '8px 24px' }}>
          <SettingRow icon="activity" title={t('История активности', 'Activity history')} sub={t('Сохранять историю входов и действий', 'Keep a record of sign-ins and actions')} right={<Toggle on={hist} onChange={setHist} label="hist" />} />
          <SettingRow icon="sparkle" title={t('Персонализация рекламы', 'Ad personalization')} sub={t('Использовать данные для рекомендаций', 'Use data for recommendations')} last right={<Toggle on={ads} onChange={setAds} label="ads" />} />
        </div>
        <div className="card" style={{ padding: '8px 24px' }}>
          <SettingRow icon="download" title={t('Скачать ваши данные', 'Download your data')} sub={t('Экспорт всех данных cotton ID', 'Export all cotton ID data')} right={<Button variant="soft" size="sm">{t('Запросить', 'Request')}</Button>} />
          <SettingRow icon="history" title={t('Журнал безопасности', 'Security log')} sub={t('Полная история событий', 'Full event history')} last right={<Button variant="soft" size="sm">{t('Открыть', 'Open')}</Button>} />
        </div>
        <div className="card card-pad" style={{ borderColor: 'var(--bad-tint)' }}>
          <div className="row gap-4" style={{ flexWrap: 'wrap' }}>
            <div style={{ flex: 1, minWidth: 220 }}>
              <h3 style={{ fontSize: 16, color: 'var(--bad)' }}>{t('Удалить аккаунт', 'Delete account')}</h3>
              <p className="faint" style={{ fontSize: 13.5, marginTop: 5, lineHeight: 1.5 }}>{t('Безвозвратно удалит cotton ID и доступ ко всем сервисам.', 'Permanently removes your cotton ID and access to all services.')}</p>
            </div>
            <Button variant="danger" icon="trash" onClick={() => setDeleteOpen(true)}>{t('Удалить', 'Delete')}</Button>
          </div>
        </div>
        {deleteOpen && <DeleteModal t={t} account={account} onChange={onChange} onClose={() => setDeleteOpen(false)} />}
      </div>
    </div>
  );
}

function DeleteModal({ t, account, onChange, onClose }: { t: Dict; account: Account; onChange: (a: Account) => void; onClose: () => void }): JSX.Element {
  const navigate = useNavigate();
  const [password, setPassword] = useState('');
  const [phrase, setPhrase] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const phraseOk = phrase.trim().toUpperCase() === t.acc_delete_confirm_word;
  const canDelete = phraseOk && password.length > 0;

  async function confirmDelete(): Promise<void> {
    if (busy || !canDelete) return; setError(null); setBusy(true);
    try { await accountApi.remove({ password }); navigate('/'); } catch (err) { setError(err instanceof ApiError && (err.status === 401 || err.status === 403) ? t.acc_pw_wrong : err instanceof ApiError ? err.message : t.err_generic); setBusy(false); }
  }

  return (
    <div role="dialog" aria-modal="true" aria-label={t.acc_delete_modal_title} style={{ position: 'fixed', inset: 0, zIndex: 80, display: 'grid', placeItems: 'center', padding: 22, background: 'var(--scrim)', backdropFilter: 'blur(6px)' }}
      onClick={(e) => { if (e.target === e.currentTarget && !busy) onClose(); }}>
      <div className="card scale-in" style={{ width: '100%', maxWidth: 440, borderRadius: 'var(--r-lg)', padding: '30px 28px' }}>
        <div className="col" style={{ gap: 16 }}>
          <div className="row" style={{ gap: 13 }}>
            <div style={{ width: 46, height: 46, borderRadius: 13, display: 'grid', placeItems: 'center', background: 'hsl(350 80% 55% / .14)', color: '#ff8da3', border: '1px solid hsl(350 80% 60% / .3)', flexShrink: 0 }}><Icon name="trash" size={22} /></div>
            <div className="col" style={{ gap: 3 }}><h3 style={{ fontSize: 19, fontWeight: 600 }}>{t.acc_delete_modal_title}</h3><p style={{ fontSize: 13.5, color: 'var(--text-3)', lineHeight: 1.45 }}>{t.acc_delete_modal_body}</p></div>
          </div>
          {error && <Notice tone="error">{error}</Notice>}
          <Field label={t.acc_delete_confirm_pw} type="password" value={password} onChange={setPassword} icon="lock" autoComplete="current-password" />
          <Field label={t.acc_delete_confirm_phrase} value={phrase} onChange={setPhrase} autoFocus />
          <div className="row" style={{ gap: 12, marginTop: 4 }}>
            <Button variant="ghost" full onClick={onClose} disabled={busy}>{t.acc_cancel}</Button>
            <Button variant="danger" full icon="trash" onClick={confirmDelete} disabled={busy || !canDelete}>{t.acc_delete_confirm_btn}</Button>
          </div>
        </div>
      </div>
    </div>
  );
}
