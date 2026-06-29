/* ============================================================================
 * Passkeys.tsx — `/passkeys` (auth-gated)
 *
 * Minimal passkey-management surface (add-passkey-auth §D6). A signed-in user can:
 *   - see their registered passkeys (nickname, when added, when last used),
 *   - add a passkey (register/begin → @github/webauthn-json create() → register/finish,
 *     prompting for a nickname), and
 *   - delete any of them.
 *
 * Auth-gating: on mount we GET /api/v1/auth/session; a 401 sends the user to /login.
 * The full account-security UI folds this in later (Change 4) reusing this API.
 * ==========================================================================*/

import { useEffect, useState } from 'react';
import { create as webauthnCreate } from '@github/webauthn-json';
import { useNavigate } from 'react-router-dom';
import { AuthChrome } from '../components/AuthChrome';
import { Button } from '../components/Button';
import { Icon } from '../components/Icon';
import { Logo } from '../components/Logo';
import { Notice } from '../components/Notice';
import { useApp } from '../i18n/context';
import type { Dict } from '../i18n/dictionary';
import { ApiError, api, passkeyApi, passkeysSupported, type Passkey } from '../lib/api';
import { isPasskeyCancellation } from '../lib/passkey';

/** Format an ISO timestamp into the active locale's short date, tolerating bad input. */
function fmtDate(iso: string | undefined, lang: string): string | null {
  if (!iso) return null;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return null;
  return d.toLocaleDateString(lang === 'ru' ? 'ru-RU' : 'en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  });
}

export function Passkeys(): JSX.Element {
  const { t, lang } = useApp();
  const navigate = useNavigate();

  const [passkeys, setPasskeys] = useState<Passkey[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const supported = passkeysSupported();

  // Auth-gate + initial load: confirm a session, then fetch the user's passkeys.
  useEffect(() => {
    const controller = new AbortController();
    api
      .session(controller.signal)
      .then(() => passkeyApi.list(controller.signal))
      .then((res) => {
        setPasskeys(res.passkeys ?? []);
        setLoaded(true);
      })
      .catch((err: unknown) => {
        if (controller.signal.aborted) return;
        if (err instanceof ApiError && err.status === 401) {
          navigate('/login');
          return;
        }
        setError(err instanceof ApiError ? err.message : t.err_generic);
        setLoaded(true);
      });
    return () => controller.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function refresh(): Promise<void> {
    const res = await passkeyApi.list();
    setPasskeys(res.passkeys ?? []);
  }

  // Register a new passkey: begin → create() → finish (with a nickname prompt).
  async function onAdd(): Promise<void> {
    if (busy || !supported) return;
    setError(null);
    setSuccess(null);

    const name = window.prompt(t.pk_name_prompt, t.pk_name_default);
    // Cancelled the nickname prompt → abort silently (no ceremony started yet).
    if (name === null) return;

    setBusy(true);
    try {
      const { publicKey } = await passkeyApi.registerBegin();
      // Runs navigator.credentials.create() with ArrayBuffer↔base64url handled.
      const credential = await webauthnCreate({ publicKey });
      await passkeyApi.registerFinish(credential, name.trim() || t.pk_name_default);
      await refresh();
      setSuccess(t.pk_added);
    } catch (err) {
      if (isPasskeyCancellation(err)) {
        setBusy(false);
        return;
      }
      setError(err instanceof ApiError ? err.message : t.pk_err_register);
    } finally {
      setBusy(false);
    }
  }

  async function onDelete(id: string): Promise<void> {
    if (busy) return;
    if (!window.confirm(t.pk_delete_confirm)) return;
    setError(null);
    setSuccess(null);
    setBusy(true);
    try {
      await passkeyApi.delete(id);
      await refresh();
      setSuccess(t.pk_deleted);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t.pk_err_delete);
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthChrome>
      <div
        className="glass rise"
        style={{ width: '100%', maxWidth: 520, borderRadius: 'var(--r-xl)', padding: '40px 38px' }}
      >
        <div className="col center" style={{ gap: 16, marginBottom: 26 }}>
          <Logo size={30} mark label={false} />
          <div className="col center" style={{ gap: 7, textAlign: 'center' }}>
            <h1
              style={{ fontFamily: 'var(--serif)', fontWeight: 400, fontSize: 32, lineHeight: 1 }}
            >
              {t.pk_title}
            </h1>
            <p style={{ fontSize: 15.5, color: 'var(--text-3)' }}>{t.pk_sub}</p>
          </div>
        </div>

        <div className="col" style={{ gap: 18 }}>
          {error && <Notice tone="error">{error}</Notice>}
          {success && <Notice tone="success">{success}</Notice>}
          {!supported && <Notice tone="info">{t.pk_unsupported}</Notice>}

          {!loaded && !error && (
            <p style={{ fontSize: 14.5, color: 'var(--text-3)', textAlign: 'center' }}>
              {t.loading}
            </p>
          )}

          {loaded && (
            <div className="col" style={{ gap: 12 }}>
              {passkeys.length === 0 ? (
                <p
                  style={{
                    fontSize: 14.5,
                    color: 'var(--text-3)',
                    textAlign: 'center',
                    padding: '8px 0',
                  }}
                >
                  {t.pk_empty}
                </p>
              ) : (
                <ul className="col" style={{ gap: 12, listStyle: 'none', padding: 0, margin: 0 }}>
                  {passkeys.map((pk) => (
                    <PasskeyRow
                      key={pk.id}
                      pk={pk}
                      t={t}
                      lang={lang}
                      busy={busy}
                      onDelete={() => onDelete(pk.id)}
                    />
                  ))}
                </ul>
              )}
            </div>
          )}

          {supported && (
            <Button size="lg" full icon="key" onClick={onAdd} disabled={busy || !loaded}>
              {t.pk_add}
            </Button>
          )}
        </div>

        <p className="center" style={{ marginTop: 22, fontSize: 14 }}>
          <a
            onClick={() => navigate('/')}
            role="button"
            style={{ color: 'var(--text-3)', cursor: 'pointer' }}
          >
            {t.pk_back}
          </a>
        </p>
      </div>
    </AuthChrome>
  );
}

/** A single passkey card: nickname + created / last-used metadata + delete. */
function PasskeyRow({
  pk,
  t,
  lang,
  busy,
  onDelete,
}: {
  pk: Passkey;
  t: Dict;
  lang: string;
  busy: boolean;
  onDelete: () => void;
}): JSX.Element {
  const created = fmtDate(pk.createdAt, lang);
  const lastUsed = fmtDate(pk.lastUsedAt, lang);

  return (
    <li
      className="row"
      style={{
        gap: 13,
        padding: '14px 16px',
        borderRadius: 'var(--r-sm)',
        background: 'var(--glass-2)',
        border: '1px solid var(--glass-border)',
      }}
    >
      <div
        style={{
          width: 42,
          height: 42,
          borderRadius: 13,
          flexShrink: 0,
          display: 'grid',
          placeItems: 'center',
          background: 'linear-gradient(140deg, var(--accent-soft), transparent)',
          border: '1px solid var(--accent-line)',
          color: 'var(--accent)',
        }}
      >
        <Icon name="key" size={20} />
      </div>
      <div className="col" style={{ gap: 3, flex: 1, minWidth: 0 }}>
        <span
          style={{
            fontSize: 15,
            fontWeight: 600,
            color: 'var(--text)',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
          }}
        >
          {pk.name || t.pk_name_default}
        </span>
        <span style={{ fontSize: 12.5, color: 'var(--text-3)' }}>
          {created && `${t.pk_created}: ${created}`}
          {created && ' · '}
          {lastUsed ? `${t.pk_last_used}: ${lastUsed}` : t.pk_never_used}
        </span>
      </div>
      <button
        type="button"
        onClick={onDelete}
        disabled={busy}
        aria-label={`${t.pk_delete}: ${pk.name || t.pk_name_default}`}
        title={t.pk_delete}
        className="row"
        style={{
          flexShrink: 0,
          width: 38,
          height: 38,
          justifyContent: 'center',
          borderRadius: 'var(--r-sm)',
          background: 'hsl(350 80% 55% / .12)',
          border: '1px solid hsl(350 80% 60% / .3)',
          color: '#ff8da3',
          cursor: busy ? 'not-allowed' : 'pointer',
          opacity: busy ? 0.6 : 1,
          transition: 'all .3s var(--ease)',
        }}
      >
        <Icon name="x" size={17} />
      </button>
    </li>
  );
}
