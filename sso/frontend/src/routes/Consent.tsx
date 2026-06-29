/* ============================================================================
 * Consent.tsx — `/consent` new design (app + scope connection)
 * ==========================================================================*/

import { useEffect, useMemo, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { Button } from '../components/Button';
import { Icon } from '../components/Icon';
import { LangSwitch } from '../components/LangSwitch';
import { Logo } from '../components/Logo';
import { Notice } from '../components/Notice';
import { Toggle } from '../components/Toggle';
import { ThemeSwitch } from '../components/ThemeSwitch';
import { useApp } from '../i18n/context';
import { ApiError, api, type ConsentInfo } from '../lib/api';
import type { Dict } from '../i18n/dictionary';

function describeScope(scope: string, t: Dict): string {
  switch (scope) {
    case 'openid': return t.scope_openid;
    case 'profile': return t.scope_profile;
    case 'email': return t.scope_email;
    case 'offline_access': return t.scope_offline_access;
    default: return scope;
  }
}

export function Consent(): JSX.Element {
  const { t } = useApp();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const consentChallenge = params.get('consent_challenge');

  const [info, setInfo] = useState<ConsentInfo | null>(null);
  const [granted, setGranted] = useState<Set<string>>(new Set());
  const [remember, setRemember] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!consentChallenge) { setLoadError(t.err_generic); return; }
    const controller = new AbortController();
    api.getConsent(consentChallenge, controller.signal)
      .then((data) => { setInfo(data); setGranted(new Set(data.requestedScopes)); })
      .catch((err: unknown) => {
        if (controller.signal.aborted) return;
        if (err instanceof ApiError && err.status === 401) { navigate('/login'); return; }
        setLoadError(err instanceof ApiError ? err.message : t.err_generic);
      });
    return () => controller.abort();
  }, [consentChallenge, navigate, t.err_generic]);

  const toggleScope = (scope: string): void => {
    if (scope === 'openid') return;
    setGranted((prev) => { const next = new Set(prev); if (next.has(scope)) next.delete(scope); else next.add(scope); return next; });
  };

  const grantList = useMemo(() => Array.from(granted), [granted]);

  async function accept(): Promise<void> {
    if (!consentChallenge || busy) return;
    setActionError(null); setBusy(true);
    try {
      const { redirectTo } = await api.acceptConsent(consentChallenge, grantList, remember);
      window.location.assign(redirectTo);
    } catch (err) { setActionError(err instanceof ApiError ? err.message : t.err_generic); setBusy(false); }
  }

  async function reject(): Promise<void> {
    if (!consentChallenge || busy) return;
    setActionError(null); setBusy(true);
    try {
      const { redirectTo } = await api.rejectConsent(consentChallenge);
      window.location.assign(redirectTo);
    } catch (err) { setActionError(err instanceof ApiError ? err.message : t.err_generic); setBusy(false); }
  }

  const appInitial = info?.client.name?.charAt(0)?.toUpperCase() || '?';

  return (
    <div className="screen-min" style={{ display: 'flex', flexDirection: 'column' }}>
      <div style={{ display: 'flex', alignItems: 'center', padding: '20px clamp(20px,5vw,40px)' }}>
        <Logo size={19} accentMark />
        <span className="spacer" />
        <div className="row gap-2"><LangSwitch /><ThemeSwitch /></div>
      </div>
      <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: '12px 20px 64px' }}>
        <div className="fade-up" style={{ width: 460, maxWidth: '100%' }}>
          {loadError && <Notice tone="error">{loadError}</Notice>}

          {info && !loadError && (
            <>
              <div className="row" style={{ justifyContent: 'center', gap: 0, marginBottom: 26 }}>
                <div style={{ width: 60, height: 60, borderRadius: 16, background: 'var(--accent)', color: '#fff', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 26, fontWeight: 700, boxShadow: 'var(--shadow-md)', zIndex: 2 }}>{appInitial}</div>
                <div style={{ display: 'flex', gap: 5, padding: '0 14px', color: 'var(--text-3)' }}>{[0, 1, 2].map((i) => <span key={i} style={{ width: 5, height: 5, borderRadius: 99, background: 'currentColor' }} />)}</div>
                <div style={{ width: 60, height: 60, borderRadius: 16, background: 'var(--surface)', border: '1px solid var(--border)', color: 'var(--accent)', display: 'flex', alignItems: 'center', justifyContent: 'center', boxShadow: 'var(--shadow-md)', zIndex: 2 }}><Icon name="mark" size={34} /></div>
              </div>
              <div style={{ textAlign: 'center', marginBottom: 22 }}>
                <h1 style={{ fontSize: 24, letterSpacing: '-0.02em' }}>{t('Войти в ', 'Sign in to ')}{info.client.name}</h1>
                <p className="muted" style={{ fontSize: 14.5, marginTop: 8, lineHeight: 1.5 }}>{t('Приложение ', '')}{info.client.name}{t(' запрашивает доступ к вашему cotton ID', ' is requesting access to your cotton ID')}</p>
              </div>

              <div className="card card-pad">
                <div className="row gap-3" style={{ paddingBottom: 16, borderBottom: '1px solid var(--border)', marginBottom: 16 }}>
                  <div className="avatar-sm" style={{ width: 40, height: 40, borderRadius: '50%', background: 'var(--accent-tint)', color: 'var(--accent)', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 16, fontWeight: 700 }}>{info.user.displayName?.charAt(0)?.toUpperCase() || '?'}</div>
                  <div><div style={{ fontWeight: 600, fontSize: 14.5 }}>{info.user.displayName || info.user.username}</div><div className="mono faint" style={{ fontSize: 12.5 }}>{info.user.email}</div></div>
                  <span className="spacer" />
                </div>

                {actionError && <Notice tone="error">{actionError}</Notice>}

                <div className="faint" style={{ fontSize: 12, fontWeight: 600, letterSpacing: '.08em', textTransform: 'uppercase', marginBottom: 12 }}>
                  {info.client.name}{t(' получит доступ к', ' will be able to')}
                </div>

                <div className="col gap-1">
                  {info.requestedScopes.map((scope, i) => {
                    const on = granted.has(scope);
                    const locked = scope === 'openid';
                    return (
                      <div key={scope} className="row gap-3" style={{ padding: '11px 0' }}>
                        <div style={{ width: 34, height: 34, flex: '0 0 auto', borderRadius: 9, background: 'var(--surface-2)', border: '1px solid var(--border)', display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--text-2)' }}>
                          <Icon name={scope === 'openid' ? 'user' : scope === 'email' ? 'mail' : scope === 'profile' ? 'user' : 'link'} size={17} />
                        </div>
                        <div style={{ flex: 1, minWidth: 0 }}>
                          <div style={{ fontSize: 14, fontWeight: 540 }}>{describeScope(scope, t)}</div>
                          <div className="faint" style={{ fontSize: 12.5, marginTop: 1 }}>{scope}</div>
                        </div>
                        {locked ? <span className="faint" style={{ fontSize: 12 }}>{t('Обязательно', 'Required')}</span> : <Toggle on={on} onChange={() => toggleScope(scope)} label={scope} />}
                      </div>
                    );
                  })}
                </div>

                <div style={{ marginTop: 12, padding: '11px 13px', borderRadius: 10, background: 'var(--surface-2)', display: 'flex', gap: 10, alignItems: 'flex-start' }}>
                  <Icon name="info" size={16} style={{ color: 'var(--text-3)', marginTop: 1, flex: '0 0 auto' }} />
                  <span className="faint" style={{ fontSize: 12.5, lineHeight: 1.5 }}>{t('Доступ можно отозвать в любой момент в разделе «Приложения» вашего cotton ID.', 'You can revoke access anytime in the Apps section of your cotton ID.')}</span>
                </div>

                <div className="row gap-3" style={{ marginTop: 20 }}>
                  <Button variant="ghost" className="btn-block" onClick={reject} disabled={busy}>{t('Отклонить', 'Deny')}</Button>
                  <Button variant="accent" className="btn-block" onClick={accept} disabled={busy}>{busy ? t('Авторизация…', 'Authorizing…') : t('Разрешить', 'Allow')}</Button>
                </div>
              </div>

              <p className="faint" style={{ textAlign: 'center', fontSize: 12, marginTop: 18, lineHeight: 1.5 }}>
                {t('Продолжая, вы соглашаетесь с условиями ', 'By continuing you agree to ')}{info.client.name}{t(' и политикой cotton ID.', '\'s terms and the cotton ID policy.')}
              </p>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
