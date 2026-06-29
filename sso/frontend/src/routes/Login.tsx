/* ============================================================================
 * Login.tsx — `/login` new design (two-step email → password/passkey)
 * ==========================================================================*/

import { useEffect, useRef, useState } from 'react';
import { get as webauthnGet } from '@github/webauthn-json';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { Button } from '../components/Button';
import { Icon } from '../components/Icon';
import { LangSwitch } from '../components/LangSwitch';
import { Logo } from '../components/Logo';
import { Notice } from '../components/Notice';
import { ThemeSwitch } from '../components/ThemeSwitch';
import { useApp } from '../i18n/context';
import { ApiError, api, passkeyApi, passkeysSupported } from '../lib/api';
import { isPasskeyCancellation } from '../lib/passkey';

function PasskeyPrompt({ open, email, onSuccess, onCancel }: { open: boolean; email: string; onSuccess: () => void; onCancel: () => void }): JSX.Element | null {
  const { t } = useApp();
  const [phase, setPhase] = useState<'scan' | 'done'>('scan');
  useEffect(() => {
    if (!open) { setPhase('scan'); return; }
    const a = setTimeout(() => setPhase('done'), 1700);
    const b = setTimeout(() => onSuccess(), 2500);
    return () => { clearTimeout(a); clearTimeout(b); };
  }, [open, onSuccess]);
  if (!open) return null;
  return (
    <div style={{ position: 'fixed', inset: 0, zIndex: 300, background: 'rgba(0,0,0,.5)', backdropFilter: 'blur(6px)', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 24, animation: 'fadeIn .2s ease both' }}>
      <div className="card scale-in" style={{ width: 380, padding: 32, textAlign: 'center', boxShadow: 'var(--shadow-lg)' }}>
        <div style={{ width: 88, height: 88, borderRadius: '50%', margin: '0 auto 22px', display: 'flex', alignItems: 'center', justifyContent: 'center', background: phase === 'done' ? 'var(--good-tint)' : 'var(--accent-tint)', color: phase === 'done' ? 'var(--good)' : 'var(--accent)', transition: 'all .4s var(--ease)' }}>
          {phase === 'done' ? <Icon name="check" size={40} stroke={2.4} /> : <div style={{ animation: 'pkpulse 1.2s var(--ease) infinite' }}><Icon name="fingerprint" size={42} stroke={1.7} /></div>}
        </div>
        <h3 style={{ fontSize: 19 }}>{phase === 'done' ? t('Готово', 'Verified') : t('Подтвердите вход', 'Verify it\'s you')}</h3>
        <p className="muted" style={{ fontSize: 14.5, marginTop: 8, lineHeight: 1.5 }}>{phase === 'done' ? t('Passkey подтверждён', 'Passkey confirmed') : t('Используйте Touch ID, Face ID или ключ безопасности', 'Use Touch ID, Face ID or a security key')}</p>
        {email && phase !== 'done' && <div className="mono" style={{ fontSize: 13, marginTop: 6, color: 'var(--text-2)' }}>{email}</div>}
        {phase !== 'done' && <button className="btn btn-quiet btn-sm" style={{ marginTop: 22 }} onClick={onCancel}>{t('Отмена', 'Cancel')}</button>}
      </div>
    </div>
  );
}

export function Login(): JSX.Element {
  const { t, lang } = useApp();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const loginChallenge = params.get('login_challenge');

  const [step, setStep] = useState<'email' | 'pw'>('email');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [show, setShow] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [passkey, setPasskey] = useState(false);
  const [passkeyBusy, setPasskeyBusy] = useState(false);
  const pwRef = useRef<HTMLInputElement>(null);

  useEffect(() => { if (step === 'pw' && pwRef.current) pwRef.current.focus(); }, [step]);

  const canPasskey = passkeysSupported();

  function submitEmail(e: React.FormEvent): void {
    e.preventDefault();
    if (!/.+@.+\..+/.test(email)) { setError(t('Введите корректный email', 'Enter a valid email')); return; }
    setError(null); setStep('pw');
  }

  async function submitPw(e: React.FormEvent): Promise<void> {
    e.preventDefault();
    if (submitting) return;
    if (password.length < 1) { setError(t('Введите пароль', 'Enter your password')); return; }
    setError(null);
    setSubmitting(true);
    try {
      await api.login({ email, password, remember: true });
      if (loginChallenge) {
        const { redirectTo } = await api.acceptLogin(loginChallenge);
        window.location.assign(redirectTo);
        return;
      }
      navigate('/');
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t.err_generic);
      setSubmitting(false);
    }
  }

  async function onPasskey(): Promise<void> {
    if (passkeyBusy || submitting) return;
    setError(null);
    setPasskey(true);
    setPasskeyBusy(true);
    try {
      const { publicKey } = await passkeyApi.loginBegin(email.trim() || undefined, loginChallenge ?? undefined);
      const credential = await webauthnGet({ publicKey });
      const result = await passkeyApi.loginFinish(credential);
      if (loginChallenge && result.redirectTo) { window.location.assign(result.redirectTo); return; }
      navigate('/');
    } catch (err) {
      if (isPasskeyCancellation(err)) { setPasskey(false); setPasskeyBusy(false); return; }
      setError(err instanceof ApiError ? err.message : t.pk_err_login);
      setPasskey(false);
      setPasskeyBusy(false);
    }
  }

  async function onCancel(): Promise<void> {
    if (!loginChallenge || submitting) return;
    setSubmitting(true);
    try {
      const { redirectTo } = await api.rejectLogin(loginChallenge);
      window.location.assign(redirectTo);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t.err_generic);
      setSubmitting(false);
    }
  }

  return (
    <div className="screen-min" style={{ display: 'flex', flexDirection: 'column', position: 'relative' }}>
      <div style={{ position: 'absolute', inset: 0, backgroundImage: 'radial-gradient(var(--grid) 1px, transparent 1px)', backgroundSize: '26px 26px', maskImage: 'radial-gradient(ellipse 70% 50% at 50% 0%, #000, transparent 75%)', WebkitMaskImage: 'radial-gradient(ellipse 70% 50% at 50% 0%, #000, transparent 75%)' }} />
      <div style={{ position: 'relative', display: 'flex', alignItems: 'center', padding: '20px clamp(20px,5vw,40px)' }}>
        <button className="btn btn-quiet btn-sm" onClick={() => navigate('/')}><Icon name="arrow-left" size={16} /> {t('На главную', 'Home')}</button>
        <span className="spacer" />
        <div className="row gap-2"><LangSwitch /><ThemeSwitch /></div>
      </div>
      <div style={{ position: 'relative', flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: '24px 20px 64px' }}>
        <div className="fade-up" style={{ width: 432, maxWidth: '100%' }}>
          <div style={{ textAlign: 'center', marginBottom: 28 }}>
            <div style={{ display: 'inline-flex', color: 'var(--accent)', marginBottom: 16 }}><Icon name="mark" size={40} /></div>
            <h1 style={{ fontSize: 27 }}>{t('Вход в cotton', 'Sign in to cotton')}</h1>
            <p className="muted" style={{ fontSize: 15, marginTop: 8 }}>{t('Один аккаунт для всех сервисов', 'One account for all services')}</p>
          </div>

          <div className="card card-pad">
            {step === 'email' ? (
              <form onSubmit={submitEmail} className="col gap-4">
                {error && <Notice tone="error">{error}</Notice>}
                <div className="field">
                  <label className="field-label">{t('Email', 'Email')}</label>
                  <div className="input-wrap">
                    <Icon name="mail" size={18} className="input-icon" />
                    <input className="input has-icon" type="text" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="name@cotton.app" autoFocus />
                  </div>
                </div>
                <Button variant="accent" className="btn-block" type="submit" iconRight="arrow-right">{t('Продолжить', 'Continue')}</Button>
                <div className="row" style={{ gap: 12, margin: '2px 0' }}>
                  <span className="divider" style={{ flex: 1 }} />
                  <span className="faint" style={{ fontSize: 12.5 }}>{t('или', 'or')}</span>
                  <span className="divider" style={{ flex: 1 }} />
                </div>
                {canPasskey && <Button variant="soft" className="btn-block" icon="fingerprint" type="button" onClick={() => setPasskey(true)}>{t('Войти по passkey', 'Sign in with passkey')}</Button>}
              </form>
            ) : (
              <form onSubmit={submitPw} className="col gap-4">
                {error && <Notice tone="error">{error}</Notice>}
                <button type="button" onClick={() => { setStep('email'); setError(null); }} className="row gap-3" style={{ background: 'var(--surface-2)', border: '1px solid var(--border)', borderRadius: 'var(--r-pill)', padding: '6px 8px 6px 6px', cursor: 'pointer', alignSelf: 'flex-start', maxWidth: '100%' }}>
                  <div className="avatar-sm" style={{ width: 28, height: 28, borderRadius: '50%', background: 'var(--accent-tint)', color: 'var(--accent)', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 12, fontWeight: 700 }}>{email[0].toUpperCase()}</div>
                  <span className="mono" style={{ fontSize: 13, color: 'var(--text-2)' }}>{email}</span>
                  <Icon name="chevron-down" size={15} style={{ color: 'var(--text-3)' }} />
                </button>
                <div className="field">
                  <label className="field-label">{t('Пароль', 'Password')}</label>
                  <div className="input-wrap">
                    <Icon name="lock" size={18} className="input-icon" />
                    <input ref={pwRef} className="input has-icon has-suffix" type={show ? 'text' : 'password'} value={password} onChange={(e) => setPassword(e.target.value)} placeholder="••••••••" autoComplete="current-password" />
                    <button type="button" className="btn btn-quiet btn-icon btn-sm" onClick={() => setShow(s => !s)} tabIndex={-1} style={{ position: 'absolute', right: 6, top: '50%', transform: 'translateY(-50%)' }}><Icon name={show ? 'eye-off' : 'eye'} size={17} /></button>
                  </div>
                </div>
                <div className="row">
                  <a className="link" style={{ fontSize: 13.5, cursor: 'pointer' }} onClick={() => navigate(loginChallenge ? `/forgot?login_challenge=${loginChallenge}` : '/forgot')}>{t('Забыли пароль?', 'Forgot password?')}</a>
                  <span className="spacer" />
                  {canPasskey && <button type="button" className="btn btn-quiet btn-sm" onClick={() => setPasskey(true)}><Icon name="fingerprint" size={16} /> {t('Passkey', 'Passkey')}</button>}
                </div>
                <Button variant="accent" className="btn-block" type="submit" disabled={submitting}>{t('Войти', 'Sign in')}</Button>
              </form>
            )}
          </div>

          <p className="muted" style={{ textAlign: 'center', fontSize: 14.5, marginTop: 22 }}>
            {t('Нет аккаунта? ', 'No account? ')}
            <a className="link" style={{ cursor: 'pointer' }} onClick={() => navigate(loginChallenge ? `/signup?login_challenge=${loginChallenge}` : '/signup')}>{t('Создать cotton ID', 'Create cotton ID')}</a>
          </p>

          {loginChallenge && (
            <p className="center" style={{ marginTop: 14, fontSize: 13.5 }}>
              <a onClick={onCancel} role="button" aria-disabled={submitting} style={{ color: 'var(--text-3)', cursor: submitting ? 'not-allowed' : 'pointer' }}>
                {lang === 'ru' ? 'Отменить вход' : 'Cancel sign-in'}
              </a>
            </p>
          )}
        </div>
      </div>
      <PasskeyPrompt open={passkey} email={email} onSuccess={() => { setPasskey(false); navigate('/'); }} onCancel={() => setPasskey(false)} />
    </div>
  );
}
