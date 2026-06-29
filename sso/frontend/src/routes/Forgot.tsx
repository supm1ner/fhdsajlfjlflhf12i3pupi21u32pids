/* ============================================================================
 * Forgot.tsx — `/forgot`
 * POST /api/v1/auth/password/forgot { email } → 202 (never enumerates).
 * Always shows the same success message regardless of whether the email exists.
 * ==========================================================================*/

import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { AuthChrome } from '../components/AuthChrome';
import { Button } from '../components/Button';
import { Field } from '../components/Field';
import { Logo } from '../components/Logo';
import { Notice } from '../components/Notice';
import { useApp } from '../i18n/context';
import { ApiError, api } from '../lib/api';

export function Forgot(): JSX.Element {
  const { t } = useApp();
  const navigate = useNavigate();
  const [email, setEmail] = useState('');
  const [sent, setSent] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function onSubmit(e: React.FormEvent): Promise<void> {
    e.preventDefault();
    if (submitting) return;
    setError(null);
    setSubmitting(true);
    try {
      await api.forgotPassword(email);
      // Non-enumerating: same outcome whether or not the account exists.
      setSent(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t.err_generic);
    } finally {
      setSubmitting(false);
    }
  }

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
              {t.forgot_title}
            </h1>
            <p style={{ fontSize: 15.5, color: 'var(--text-3)' }}>{t.forgot_sub}</p>
          </div>
        </div>

        {sent ? (
          <div className="col" style={{ gap: 20 }}>
            <Notice tone="success">{t.forgot_sent}</Notice>
            <Button variant="glass" size="lg" full onClick={() => navigate('/login')}>
              {t.back_to_login}
            </Button>
          </div>
        ) : (
          <form className="col" style={{ gap: 18 }} onSubmit={onSubmit}>
            {error && <Notice tone="error">{error}</Notice>}
            <Field
              label={t.f_email}
              type="email"
              icon="mail"
              name="email"
              autoComplete="email"
              value={email}
              onChange={setEmail}
              required
              autoFocus
            />
            <Button size="lg" full type="submit" iconRight="arrow" disabled={submitting}>
              {t.forgot_btn}
            </Button>
            <p className="center" style={{ fontSize: 14.5, color: 'var(--text-3)' }}>
              <a
                onClick={() => navigate('/login')}
                style={{ color: 'var(--accent)', fontWeight: 700, cursor: 'pointer' }}
              >
                {t.back_to_login}
              </a>
            </p>
          </form>
        )}
      </div>
    </AuthChrome>
  );
}
