/* ============================================================================
 * Landing.tsx — `/` new design (LandingB variant)
 * ==========================================================================*/

import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Avatar } from '../components/Avatar';
import { Badge } from '../components/Badge';
import { Button } from '../components/Button';
import { CottonMark, Icon, type IconName } from '../components/Icon';
import { LangSwitch } from '../components/LangSwitch';
import { Logo } from '../components/Logo';
import { ThemeSwitch } from '../components/ThemeSwitch';
import { UserMenu } from '../components/UserMenu';
import { useApp } from '../i18n/context';
import type { Dict } from '../i18n/dictionary';
import { api, type User } from '../lib/api';

const SERVICES = [
  { key: 'mail', icon: 'mail-app' as IconName, ru: 'Почта', en: 'Mail', dRu: 'Письма и домены', dEn: 'Email & domains' },
  { key: 'disk', icon: 'disk' as IconName, ru: 'Диск', en: 'Disk', dRu: 'Файлы и бэкапы', dEn: 'Files & backups' },
  { key: 'music', icon: 'music' as IconName, ru: 'Музыка', en: 'Music', dRu: 'Стриминг без рекламы', dEn: 'Ad-free streaming' },
  { key: 'talk', icon: 'chat' as IconName, ru: 'Мессенджер', en: 'Messenger', dRu: 'Чаты и звонки', dEn: 'Chats & calls' },
];

function Header({ onLogin, onSignup }: { onLogin: () => void; onSignup: () => void }): JSX.Element {
  const { t } = useApp();
  const [user, setUser] = useState<User | null | undefined>(undefined);
  const navigate = useNavigate();

  useEffect(() => {
    const controller = new AbortController();
    api.session(controller.signal)
      .then((res) => setUser(res.user))
      .catch(() => { if (!controller.signal.aborted) setUser(null); });
    return () => controller.abort();
  }, []);

  return (
    <header style={{
      position: 'sticky', top: 0, zIndex: 40, display: 'flex', alignItems: 'center', gap: 16,
      padding: '18px clamp(20px, 5vw, 64px)',
      background: 'color-mix(in srgb, var(--bg) 80%, transparent)',
      backdropFilter: 'blur(14px)', borderBottom: '1px solid var(--border)',
    }}>
      <Logo size={21} accentMark />
      <span className="spacer" />
      <nav className="landing-nav row gap-2" style={{ marginRight: 4 }}>
        {['Сервисы;Services', 'Безопасность;Security', 'Разработчикам;Developers'].map((s) => {
          const [ru, en] = s.split(';');
          return <a key={ru} className="btn btn-quiet btn-sm" href="#">{t(ru, en)}</a>;
        })}
      </nav>
      <LangSwitch />
      <ThemeSwitch />
      {user ? (
        <UserMenu user={user} />
      ) : (
        <>
          <Button variant="ghost" size="sm" onClick={onLogin}>{t('Войти', 'Sign in')}</Button>
          <Button variant="accent" size="sm" onClick={onSignup}>{t('Создать аккаунт', 'Create account')}</Button>
        </>
      )}
    </header>
  );
}

function Footer(): JSX.Element {
  const navigate = useNavigate();
  const { t } = useApp();
  const cols = [
    { h: t('Сервисы', 'Services'), items: ['cotton Почта', 'cotton Диск', 'cotton Музыка', 'cotton Talk'] },
    { h: t('Аккаунт', 'Account'), items: [t('Вход', 'Sign in'), t('Безопасность', 'Security'), t('Устройства', 'Devices'), t('Приватность', 'Privacy')] },
    { h: t('Разработчикам', 'Developers'), items: ['OAuth 2.0', 'OpenID Connect', 'API', t('Документация', 'Docs')] },
    { h: t('Компания', 'Company'), items: [t('О cotton', 'About'), t('Поддержка', 'Support'), t('Статус', 'Status'), t('Контакты', 'Contacts')] },
  ];
  return (
    <footer style={{ borderTop: '1px solid var(--border)', padding: 'clamp(40px,6vw,72px) clamp(20px,5vw,64px) 40px', background: 'var(--bg-sunken)' }}>
      <div style={{ maxWidth: 1180, margin: '0 auto', display: 'grid', gridTemplateColumns: '1.4fr repeat(4, 1fr)', gap: 32 }}>
        <div>
          <Logo size={20} accentMark />
          <p className="faint" style={{ marginTop: 14, fontSize: 13.5, maxWidth: 230, lineHeight: 1.6 }}>
            {t('Единый вход в экосистему cotton.', 'Single sign-on for the cotton ecosystem.')}
          </p>
        </div>
        {cols.map((c) => (
          <div key={c.h} className="col gap-3">
            <div style={{ fontSize: 13, fontWeight: 600, marginBottom: 4 }}>{c.h}</div>
            {c.items.map((i) => (
              <a key={i} href="#" className="faint" style={{ fontSize: 13.5, lineHeight: 1.4 }} onClick={(e) => { e.preventDefault(); if (i === t('Документация', 'Docs') || i === 'Docs') navigate('/docs'); }} role="link">{i}</a>
            ))}
          </div>
        ))}
      </div>
      <div style={{ maxWidth: 1180, margin: '40px auto 0', paddingTop: 24, borderTop: '1px solid var(--border)', display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 12 }}>
        <span className="faint" style={{ fontSize: 13 }}>© 2026 cotton</span>
        <span className="faint" style={{ fontSize: 13 }}>{t('Сделано в духе чистоты и простоты', 'Crafted for clarity')}</span>
      </div>
    </footer>
  );
}

function ServiceTile({ s, big }: { s: typeof SERVICES[number]; big?: boolean }): JSX.Element {
  const { t } = useApp();
  const [h, setH] = useState(false);
  return (
    <div onMouseEnter={() => setH(true)} onMouseLeave={() => setH(false)}
      className="card" style={{
        padding: big ? 24 : 20, display: 'flex', flexDirection: 'column', gap: big ? 16 : 12,
        cursor: 'pointer', transition: 'transform .25s var(--ease), box-shadow .25s var(--ease), border-color .25s',
        transform: h ? 'translateY(-3px)' : 'none', boxShadow: h ? 'var(--shadow-md)' : 'var(--shadow-sm)',
        borderColor: h ? 'var(--border-strong)' : 'var(--border)',
      }}>
      <div style={{ width: big ? 50 : 42, height: big ? 50 : 42, borderRadius: 13, display: 'flex',
        alignItems: 'center', justifyContent: 'center', background: h ? 'var(--accent)' : 'var(--surface-2)',
        color: h ? 'var(--accent-fg)' : 'var(--text)', border: '1px solid var(--border)', transition: 'all .25s var(--ease)' }}>
        <Icon name={s.icon} size={big ? 24 : 21} />
      </div>
      <div>
        <div style={{ fontWeight: 600, fontSize: big ? 18 : 15.5, letterSpacing: '-0.01em' }}>{t(s.ru, s.en)}</div>
        <div className="faint" style={{ fontSize: 13.5, marginTop: 3 }}>{t(s.dRu, s.dEn)}</div>
      </div>
    </div>
  );
}

function AccountHubMock({ t }: { t: Dict }): JSX.Element {
  return (
    <div className="card" style={{ boxShadow: 'var(--shadow-lg)', overflow: 'hidden', width: '100%' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '13px 16px', borderBottom: '1px solid var(--border)' }}>
        <div className="row gap-2">{ [0,1,2].map(i => <span key={i} style={{ width: 11, height: 11, borderRadius: 99, background: 'var(--border-strong)' }} />) }</div>
        <span className="mono faint" style={{ fontSize: 12, marginLeft: 8 }}>id.cotton.app</span>
        <span className="spacer" />
        <Icon name="lock" size={13} style={{ color: 'var(--text-3)' }} />
      </div>
      <div style={{ padding: 22 }}>
        <div className="row gap-3" style={{ marginBottom: 18 }}>
          <Avatar name="Алия Жанибек" size={46} />
          <div>
            <div style={{ fontWeight: 600, fontSize: 15.5 }}>{t('Алия Жанибек', 'Aliya Zhanibek')}</div>
            <div className="mono faint" style={{ fontSize: 12.5 }}>aliya@cotton.app</div>
          </div>
          <span className="spacer" />
          <Badge tone="good" dot>{t('Защищён', 'Secured')}</Badge>
        </div>
        <div className="faint" style={{ fontSize: 11.5, fontWeight: 600, letterSpacing: '.1em', textTransform: 'uppercase', marginBottom: 10 }}>{t('Подключённые сервисы', 'Connected services')}</div>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4,1fr)', gap: 10, marginBottom: 18 }}>
          {SERVICES.map(s => (
            <div key={s.key} style={{ aspectRatio: '1', borderRadius: 12, border: '1px solid var(--border)', background: 'var(--surface-2)', display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 6 }}>
              <Icon name={s.icon} size={20} />
              <span style={{ fontSize: 10.5, color: 'var(--text-2)' }}>{t(s.ru, s.en)}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

export function Landing(): JSX.Element {
  const { t } = useApp();
  const navigate = useNavigate();

  return (
    <div>
      <Header onLogin={() => navigate('/login')} onSignup={() => navigate('/signup')} />

      <section style={{ padding: 'clamp(48px,7vw,96px) clamp(20px,5vw,64px) clamp(40px,5vw,72px)' }}>
        <div style={{ maxWidth: 1180, margin: '0 auto', display: 'grid', gridTemplateColumns: 'minmax(0,1fr) minmax(0,1fr)', gap: 'clamp(32px,5vw,72px)', alignItems: 'center' }}>
          <div className="fade-up" style={{ minWidth: 0 }}>
            <div className="badge badge-accent" style={{ marginBottom: 22 }}><span className="dot" /> {t('cotton ID · единый аккаунт', 'cotton ID · single account')}</div>
            <h1 style={{ fontSize: 'clamp(38px,5.4vw,64px)', letterSpacing: '-0.04em', lineHeight: 1.05 }}>
              {t('Ваш аккаунт', 'Your account')}<br />
              {t('для всего ', 'for the whole ')}<span style={{ color: 'var(--accent)' }}>cotton.</span>
            </h1>
            <p className="muted" style={{ fontSize: 'clamp(16px,1.6vw,19px)', lineHeight: 1.55, marginTop: 22, maxWidth: 480, textWrap: 'pretty' }}>
              {t('Один вход для Почты, Диска, Музыки и Мессенджера. Управляйте безопасностью, устройствами и доступом приложений из одной панели.',
                'One sign-in for Mail, Disk, Music and Messenger. Manage security, devices and app access from a single panel.')}
            </p>
            <div className="row gap-3" style={{ marginTop: 32 }}>
              <Button variant="accent" size="lg" iconRight="arrow-right" onClick={() => navigate('/signup')}>{t('Создать cotton ID', 'Create cotton ID')}</Button>
              <Button variant="ghost" size="lg" onClick={() => navigate('/login')}>{t('Войти', 'Sign in')}</Button>
            </div>
            <div className="row gap-4" style={{ marginTop: 30, flexWrap: 'wrap' }}>
              {[[t('5+ сервисов', '5+ services'), 'grid' as IconName], [t('2 фактора', '2-factor'), 'shield-check' as IconName], [t('Passkey', 'Passkey'), 'fingerprint' as IconName]].map(([a, ic]) => (
                <span key={a as string} className="row gap-2 faint" style={{ fontSize: 13.5 }}><Icon name={ic} size={16} />{a as string}</span>
              ))}
            </div>
          </div>
          <div className="scale-in" style={{ position: 'relative', minWidth: 0 }}>
            <div style={{ position: 'absolute', inset: '-12% -8%', background: 'radial-gradient(circle at 70% 30%, var(--accent-tint-2), transparent 60%)', filter: 'blur(8px)' }} />
            <div style={{ position: 'relative' }}><AccountHubMock t={t} /></div>
          </div>
        </div>
      </section>

      <section style={{ padding: 'clamp(40px,6vw,80px) clamp(20px,5vw,64px)', borderTop: '1px solid var(--border)', background: 'var(--bg-sunken)' }}>
        <div style={{ maxWidth: 1180, margin: '0 auto' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-end', marginBottom: 28, flexWrap: 'wrap', gap: 12 }}>
            <div>
              <div className="eyebrow" style={{ marginBottom: 8 }}>{t('Экосистема', 'Ecosystem')}</div>
              <h2 style={{ fontSize: 'clamp(26px,3.2vw,38px)' }}>{t('Один аккаунт — все сервисы', 'One account, every service')}</h2>
            </div>
          </div>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px,1fr))', gap: 14 }}>
            {SERVICES.map(s => <ServiceTile key={s.key} s={s} />)}
          </div>
        </div>
      </section>

      <section style={{ padding: 'clamp(48px,7vw,88px) clamp(20px,5vw,64px)' }}>
        <div style={{ maxWidth: 1180, margin: '0 auto', padding: 'clamp(26px,5vw,56px)', borderRadius: 'var(--r-xl)', display: 'grid', gridTemplateColumns: '1.2fr 1fr', gap: 40, alignItems: 'center', background: '#0e0e13', color: '#fafafa', border: '1px solid rgba(255,255,255,0.10)', boxShadow: 'var(--shadow-lg)' }}>
          <div style={{ minWidth: 0 }}>
            <div style={{ display: 'inline-flex', color: '#9b7bf2', marginBottom: 18 }}><Icon name="key" size={26} /></div>
            <h2 style={{ fontSize: 'clamp(24px,3vw,36px)', color: '#fafafa' }}>{t('Войти через cotton', 'Sign in with cotton')}</h2>
            <p style={{ color: 'rgba(255,255,255,.66)', fontSize: 16, lineHeight: 1.55, marginTop: 14, maxWidth: 420 }}>{t('OAuth 2.0 и OpenID Connect для вашего приложения. Подключите вход cotton за час.', 'OAuth 2.0 and OpenID Connect for your app. Add cotton sign-in in an hour.')}</p>
            <div style={{ marginTop: 26 }}>
              <button className="btn btn-lg" style={{ background: '#fff', color: '#0e0e13' }} onClick={() => navigate('/docs')}>{t('Документация', 'Read the docs')} <Icon name="arrow-right" size={18} /></button>
            </div>
          </div>
          <div className="mono" style={{ fontSize: 13, lineHeight: 1.8, background: 'rgba(255,255,255,.05)', border: '1px solid rgba(255,255,255,.12)', borderRadius: 14, padding: 18, color: '#e7e7ec', overflowX: 'auto', whiteSpace: 'nowrap', minWidth: 0 }}>
            <div style={{ color: 'rgba(255,255,255,.42)' }}>// OpenID Connect</div>
            <div><span style={{ color: '#a78bfa' }}>GET</span> /authorize</div>
            <div style={{ color: 'rgba(255,255,255,.8)' }}>  client_id=<span style={{ color: '#7dd3fc' }}>cotton_a1b2</span></div>
            <div style={{ color: 'rgba(255,255,255,.8)' }}>  scope=<span style={{ color: '#7dd3fc' }}>openid profile email</span></div>
            <div style={{ color: 'rgba(255,255,255,.8)' }}>  redirect_uri=<span style={{ color: '#7dd3fc' }}>https://app/cb</span></div>
            <div style={{ marginTop: 8, color: 'rgba(255,255,255,.42)' }}>// → cotton ID consent → code</div>
          </div>
        </div>
      </section>

      <Footer />
    </div>
  );
}
