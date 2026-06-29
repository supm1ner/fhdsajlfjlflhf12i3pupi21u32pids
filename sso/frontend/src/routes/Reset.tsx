/* ============================================================================
 * Reset.tsx — `/reset?token=...`
 * POST /api/v1/auth/password/reset { token, password } → 204.
 * Reads the single-use token from the query string; shows the strength meter.
 * ==========================================================================*/

import { useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { AuthChrome } from '../components/AuthChrome';
import { Button } from '../components/Button';
import { Field } from '../components/Field';
import { Icon } from '../components/Icon';
import { Logo } from '../components/Logo';
import { Notice } from '../components/Notice';
import { PasswordStrength } from '../components/PasswordStrength';
import { useApp } from '../i18n/context';
import { ApiError, api } from '../lib/api';

export function Reset(): JSX.Element {
  const { t } = useApp();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const token = params.get('token') ?? '';

  const [password, setPassword] = useState('');
  const [show, setShow] = useState(false);
  const [done, setDone] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function onSubmit(e: React.FormEvent): Promise<void> {
    e.preventDefault();
    if (submitting) return;
    setError(null);
    setSubmitting(true);
    try {
      await api.resetPassword(token, password);
      setDone(true);
    } catch (err) {
      // A bad/expired token surfaces as a 4xx; show the dedicated copy.
      if (err instanceof ApiError && (err.status === 400 || err.status === 404 || err.status === 410)) {
        setError(t.reset_bad_token);
      } else {
        setError(err instanceof ApiError ? err.message : t.err_generic);
      }
    } finally {
      setSubmitting(false);
    }
  }

  const missingToken = !token;

  return (
    <AuthChrome>
      <div
        className="glass rise"
        style={{ width: '100%', maxWidth: 440, borderRadius: 'var(--r-xl)', padding: '40px 38px' }}
      >
        <div className="col center" style={{ gap: 16, marginBottom: 28 }}>
          <Logo size={30} mark label={false} />
          <div className="col center" style={{ gap: 7, textAlign: 'center' }}>
            <h1 style={{ fontFamily: 'var(--serif)', fontWeight: 400, fontSize: 32, lineHeight: 1 }}>
              {t.reset_title}
            </h1>
            <p style={{ fontSize: 15.5, color: 'var(--text-3)' }}>{t.reset_sub}</p>
          </div>
        </div>

        {done ? (
          <div className="col" style={{ gap: 20 }}>
            <Notice tone="success">{t.reset_done}</Notice>
            <Button size="lg" full onClick={() => navigate('/login')}>
              {t.back_to_login}
            </Button>
          </div>
        ) : missingToken ? (
          <div className="col" style={{ gap: 20 }}>
            <Notice tone="error">{t.reset_bad_token}</Notice>
            <Button variant="glass" size="lg" full onClick={() => navigate('/forgot')}>
              {t.forgot_title}
            </Button>
          </div>
        ) : (
          <form className="col" style={{ gap: 18 }} onSubmit={onSubmit}>
            {error && <Notice tone="error">{error}</Notice>}
            <Field
              label={t.f_password}
              type={show ? 'text' : 'password'}
              icon="lock"
              name="password"
              autoComplete="new-password"
              value={password}
              onChange={setPassword}
              required
              autoFocus
              right={
                <button
                  type="button"
                  aria-label={show ? 'Hide password' : 'Show password'}
                  onClick={() => setShow(!show)}
                  style={{ color: 'var(--text-3)', display: 'grid' }}
                >
                  <Icon name="eye" size={18} />
                </button>
              }
            />
            <PasswordStrength password={password} t={t} />
            <Button size="lg" full type="submit" iconRight="check" disabled={submitting}>
              {t.reset_btn}
            </Button>
          </form>
        )}
      </div>
    </AuthChrome>
  );
}
