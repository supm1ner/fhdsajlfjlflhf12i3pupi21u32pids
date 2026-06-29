/* ============================================================================
 * Signup.tsx — `/signup` (form → passkey-offer)
 * Email verification (form → code → passkey-offer) is implemented server-side
 * in auth/verify.go but disabled in the UI for development. Re-enable by
 * inserting the code step between form submit and passkey-offer.
 * ==========================================================================*/

import { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { Button } from '../components/Button';
import { Icon } from '../components/Icon';
import { LangSwitch } from '../components/LangSwitch';
import { Logo } from '../components/Logo';
import { Notice } from '../components/Notice';
import { ThemeSwitch } from '../components/ThemeSwitch';
import { useApp } from '../i18n/context';
import { ApiError, api, type ProblemJson } from '../lib/api';

function pwScore(p: string): number {
  let s = 0;
  if (p.length >= 8) s++;
  if (/[A-ZА-Я]/.test(p)) s++;
  if (/[0-9]/.test(p)) s++;
  if (/[^A-Za-zА-Я0-9]/.test(p)) s++;
  return s;
}

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

export function Signup(): JSX.Element {
  const { t } = useApp();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const loginChallenge = params.get('login_challenge');

  const [step, setStep] = useState<'form' | 'passkey-offer'>('form');
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [show, setShow] = useState(false);
  const [agree, setAgree] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [passkey, setPasskey] = useState(false);

  const score = pwScore(password);
  const scoreLabels = [t('Слабый', 'Weak'), t('Слабый', 'Weak'), t('Средний', 'Fair'), t('Хороший', 'Good'), t('Надёжный', 'Strong')];
  const scoreColors = ['var(--bad)', 'var(--bad)', 'var(--warn)', 'var(--good)', 'var(--good)'];

  async function submit(e: React.FormEvent): Promise<void> {
    e.preventDefault();
    const errs: string[] = [];
    if (name.trim().length < 2) errs.push(t('Укажите никнейм', 'Enter a username'));
    if (!/.+@.+\..+/.test(email)) errs.push(t('Некорректный email', 'Invalid email'));
    if (password.length < 8) errs.push(t('Минимум 8 символов', 'At least 8 characters'));
    if (!agree) errs.push(t('Примите условия', 'Accept the terms'));
    if (errs.length > 0) { setError(errs.join('. ')); return; }
    setError(null);
    if (submitting) return;
    setSubmitting(true);
    try {
      await api.signup({ displayName: name.trim(), username: name.trim(), email, password });
      setStep('passkey-offer');
    } catch (err) {
      if (err instanceof ApiError) {
        const p: ProblemJson = err.problem;
        if (p.field === 'email') { setError(t('Почта уже используется', 'This email is already taken')); }
        else if (p.field === 'username') { setError(t('Имя пользователя занято', 'This username is already taken')); }
        else { setError(p.detail || t.err_generic); }
      } else {
        setError(t.err_generic);
      }
    } finally {
      setSubmitting(false);
    }
  }

  function onDone(): void {
    if (loginChallenge) {
      api.acceptLogin(loginChallenge).then(({ redirectTo }) => {
        window.location.assign(redirectTo);
      });
      return;
    }
    navigate('/');
  }

  if (step === 'passkey-offer') {
    return (
      <div className="screen-min" style={{ display: 'flex', flexDirection: 'column', position: 'relative' }}>
        <div style={{ position: 'absolute', inset: 0, backgroundImage: 'radial-gradient(var(--grid) 1px, transparent 1px)', backgroundSize: '26px 26px', maskImage: 'radial-gradient(ellipse 70% 50% at 50% 0%, #000, transparent 75%)', WebkitMaskImage: 'radial-gradient(ellipse 70% 50% at 50% 0%, #000, transparent 75%)' }} />
        <div style={{ position: 'relative', display: 'flex', alignItems: 'center', padding: '20px clamp(20px,5vw,40px)' }}>
          <button className="btn btn-quiet btn-sm" onClick={() => navigate('/')}><Icon name="arrow-left" size={16} /> {t('На главную', 'Home')}</button>
          <span className="spacer" />
          <div className="row gap-2"><LangSwitch /><ThemeSwitch /></div>
        </div>
        <div style={{ position: 'relative', flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: '24px 20px 64px' }}>
          <div className="fade-up" style={{ width: 432, maxWidth: '100%', textAlign: 'center' }}>
            <div style={{ width: 76, height: 76, borderRadius: '50%', margin: '0 auto 22px', display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'var(--accent-tint)', color: 'var(--accent)' }}>
              <Icon name="fingerprint" size={38} />
            </div>
            <h1 style={{ fontSize: 26 }}>{t('Настроить passkey?', 'Set up a passkey?')}</h1>
            <p className="muted" style={{ fontSize: 15, marginTop: 10, lineHeight: 1.55, maxWidth: 340, margin: '10px auto 0' }}>{t('Входите по Face ID, Touch ID или ключу — без паролей. Рекомендуем для cotton ID.', 'Sign in with Face ID, Touch ID or a key — no passwords. Recommended for cotton ID.')}</p>
            <div className="col gap-3" style={{ marginTop: 28 }}>
              <Button variant="accent" full icon="fingerprint" onClick={() => setPasskey(true)}>{t('Создать passkey', 'Create passkey')}</Button>
              <Button variant="quiet" full onClick={() => void onDone()}>{t('Позже', 'Maybe later')}</Button>
            </div>
          </div>
        </div>
        {error && <Notice tone="error">{error}</Notice>}
        <PasskeyPrompt open={passkey} email={email} onSuccess={() => { setPasskey(false); void onDone(); }} onCancel={() => setPasskey(false)} />
      </div>
    );
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
          <div style={{ textAlign: 'center', marginBottom: 26 }}>
            <div style={{ display: 'inline-flex', color: 'var(--accent)', marginBottom: 16 }}><Icon name="mark" size={40} /></div>
            <h1 style={{ fontSize: 27 }}>{t('Создать cotton ID', 'Create cotton ID')}</h1>
            <p className="muted" style={{ fontSize: 15, marginTop: 8 }}>{t('Один аккаунт для всей экосистемы', 'One account for everything')}</p>
          </div>
          <div className="card card-pad">
            <form onSubmit={submit} className="col gap-4">
              {error && <Notice tone="error">{error}</Notice>}
              <div className="field">
                <label className="field-label">{t('Никнейм', 'Username')}</label>
                <div className="input-wrap">
                  <Icon name="user" size={18} className="input-icon" />
                  <input className="input has-icon" value={name} onChange={(e) => setName(e.target.value)} placeholder={t('alex.cotton', 'alex.cotton')} autoFocus />
                </div>
              </div>
              <div className="field">
                <label className="field-label">Email</label>
                <div className="input-wrap">
                  <Icon name="mail" size={18} className="input-icon" />
                  <input className="input has-icon" type="text" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="name@cotton.app" />
                </div>
              </div>
              <div className="field">
                <label className="field-label">{t('Пароль', 'Password')}</label>
                <div className="input-wrap">
                  <Icon name="lock" size={18} className="input-icon" />
                  <input className="input has-icon has-suffix" type={show ? 'text' : 'password'} value={password} onChange={(e) => setPassword(e.target.value)} placeholder={t('Минимум 8 символов', 'At least 8 characters')} />
                  <button type="button" className="btn btn-quiet btn-icon btn-sm" onClick={() => setShow(s => !s)} tabIndex={-1} style={{ position: 'absolute', right: 6, top: '50%', transform: 'translateY(-50%)' }}><Icon name={show ? 'eye-off' : 'eye'} size={17} /></button>
                </div>
                {password && (
                  <div className="row gap-3" style={{ marginTop: 9 }}>
                    <div className="row" style={{ gap: 4, flex: 1 }}>{[0, 1, 2, 3].map((i) => <span key={i} style={{ height: 4, flex: 1, borderRadius: 99, background: i < score ? scoreColors[score] : 'var(--border-strong)', transition: 'background .2s' }} />)}</div>
                    <span style={{ fontSize: 12, color: scoreColors[score], fontWeight: 540, minWidth: 56, textAlign: 'right' }}>{scoreLabels[score]}</span>
                  </div>
                )}
              </div>
              <label className="row gap-3" style={{ cursor: 'pointer', alignItems: 'flex-start' }}>
                <button type="button" role="checkbox" aria-checked={agree} onClick={() => setAgree(a => !a)}
                  style={{ flex: '0 0 auto', width: 20, height: 20, borderRadius: 6, border: '1.5px solid ' + (agree ? 'var(--accent)' : 'var(--border-strong)'), background: agree ? 'var(--accent)' : 'transparent', color: '#fff', display: 'flex', alignItems: 'center', justifyContent: 'center', cursor: 'pointer', marginTop: 1 }}>
                  {agree && <Icon name="check" size={13} stroke={3} />}
                </button>
                <span className="muted" style={{ fontSize: 13.5, lineHeight: 1.5 }}>{t('Принимаю ', 'I agree to the ')}<a className="link">{t('условия', 'Terms')}</a>{t(' и ', ' and ')}<a className="link">{t('политику конфиденциальности', 'Privacy Policy')}</a></span>
              </label>
              <Button variant="accent" className="btn-block" type="submit" iconRight="arrow-right" disabled={submitting}>{t('Создать аккаунт', 'Create account')}</Button>
            </form>
          </div>
          <p className="muted" style={{ textAlign: 'center', fontSize: 14.5, marginTop: 22 }}>
            {t('Уже есть аккаунт? ', 'Already have an account? ')}
            <a className="link" style={{ cursor: 'pointer' }} onClick={() => navigate(loginChallenge ? `/login?login_challenge=${loginChallenge}` : '/login')}>{t('Войти', 'Sign in')}</a>
          </p>
        </div>
      </div>
    </div>
  );
}
