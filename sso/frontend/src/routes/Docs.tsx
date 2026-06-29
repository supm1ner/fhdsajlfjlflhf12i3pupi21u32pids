/* ============================================================================
 * Docs.tsx — `/docs` developer documentation
 * ==========================================================================*/

import { useNavigate } from 'react-router-dom';
import { Icon } from '../components/Icon';
import { LangSwitch } from '../components/LangSwitch';
import { Logo } from '../components/Logo';
import { ThemeSwitch } from '../components/ThemeSwitch';
import { useApp } from '../i18n/context';

function Code({ children }: { children: string }): JSX.Element {
  return <code style={{ fontSize: 13, background: 'var(--surface-2)', padding: '1px 6px', borderRadius: 5, color: 'var(--accent)' }}>{children}</code>;
}

function Pre({ children }: { children: string }): JSX.Element {
  return (
    <pre style={{ fontSize: 13, lineHeight: 1.7, background: 'var(--surface-2)', border: '1px solid var(--border)', borderRadius: 10, padding: 16, overflowX: 'auto', whiteSpace: 'pre', marginTop: 8, marginBottom: 0 }}>
      {children}
    </pre>
  );
}

function Section({ title, children }: { title: string; children: JSX.Element | JSX.Element[] }): JSX.Element {
  return (
    <div style={{ marginTop: 32 }}>
      <h2 style={{ fontSize: 21, fontWeight: 600, marginBottom: 12 }}>{title}</h2>
      {children}
    </div>
  );
}

export function Docs(): JSX.Element {
  const { t } = useApp();
  const navigate = useNavigate();

  return (
    <div className="screen-min" style={{ display: 'flex', flexDirection: 'column' }}>
      <div style={{ display: 'flex', alignItems: 'center', padding: '20px clamp(20px,5vw,40px)' }}>
        <button className="btn btn-quiet btn-sm" onClick={() => navigate('/')}><Icon name="arrow-left" size={16} /> {t('На главную', 'Home')}</button>
        <span className="spacer" />
        <div className="row gap-2"><LangSwitch /><ThemeSwitch /></div>
      </div>
      <div style={{ flex: 1, display: 'flex', justifyContent: 'center', padding: '0 20px 64px' }}>
        <div className="fade-up" style={{ width: 780, maxWidth: '100%' }}>
          <div style={{ marginBottom: 12 }}><Logo size={24} accentMark /></div>
          <h1 style={{ fontSize: 32, letterSpacing: '-0.03em', lineHeight: 1.15 }}>{t('Документация разработчика', 'Developer Documentation')}</h1>
          <p className="muted" style={{ fontSize: 16, marginTop: 8, lineHeight: 1.5 }}>
            {t('Интегрируйте вход через cotton в ваше приложение с помощью OAuth 2.0 и OpenID Connect.', 'Integrate Sign in with cotton into your app using OAuth 2.0 and OpenID Connect.')}
          </p>

          <Section title={t('Быстрый старт', 'Quick Start')}>
            <p className="muted" style={{ fontSize: 14.5, lineHeight: 1.6 }}>
              {t('Зарегистрируйте своё приложение, получите client ID и настройте редирект. После этого пользователи смогут входить через cotton одной кнопкой.', 'Register your app, get a client ID and configure a redirect URI. Users can then sign in with cotton in one click.')}
            </p>
            <div className="card card-pad" style={{ marginTop: 16 }}>
              <div className="col gap-4">
                <div className="row gap-4" style={{ alignItems: 'flex-start' }}>
                  <div style={{ width: 28, height: 28, borderRadius: 99, background: 'var(--accent-tint)', color: 'var(--accent)', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0, fontSize: 13, fontWeight: 600 }}>1</div>
                  <div>
                    <div style={{ fontWeight: 600, fontSize: 15 }}>{t('Регистрация приложения', 'Register your app')}</div>
                    <p className="muted" style={{ fontSize: 14, marginTop: 4, lineHeight: 1.5 }}>
                      {t('Войдите в консоль управления cotton ID и нажмите "Создать приложение". Укажите название, redirect URI и выберите тип клиента.', 'Go to the cotton ID admin console and click "Create app". Enter a name, redirect URI and choose the client type.')}
                    </p>
                  </div>
                </div>
                <div className="row gap-4" style={{ alignItems: 'flex-start' }}>
                  <div style={{ width: 28, height: 28, borderRadius: 99, background: 'var(--accent-tint)', color: 'var(--accent)', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0, fontSize: 13, fontWeight: 600 }}>2</div>
                  <div>
                    <div style={{ fontWeight: 600, fontSize: 15 }}>{t('Настройка OAuth', 'Configure OAuth')}</div>
                    <p className="muted" style={{ fontSize: 14, marginTop: 4, lineHeight: 1.5 }}>
                      {t('Используйте endpoint /authorize для перенаправления пользователя и получите code после подтверждения.', 'Use the /authorize endpoint to redirect the user and receive a code after they grant consent.')}
                    </p>
                  </div>
                </div>
                <div className="row gap-4" style={{ alignItems: 'flex-start' }}>
                  <div style={{ width: 28, height: 28, borderRadius: 99, background: 'var(--accent-tint)', color: 'var(--accent)', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0, fontSize: 13, fontWeight: 600 }}>3</div>
                  <div>
                    <div style={{ fontWeight: 600, fontSize: 15 }}>{t('Обмен кода на токен', 'Exchange code for token')}</div>
                    <p className="muted" style={{ fontSize: 14, marginTop: 4, lineHeight: 1.5 }}>
                      {t('Отправьте code на /token, получите access и id токены. Проверьте id токен — в нём email и username пользователя.', 'Send the code to /token to receive access and id tokens. Verify the id token — it contains the user\'s email and username.')}
                    </p>
                  </div>
                </div>
              </div>
            </div>
          </Section>

          <Section title={t('Endpoints', 'Endpoints')}>
            <p className="muted" style={{ fontSize: 14.5, lineHeight: 1.6 }}>
              {t('Все запросы направляются на базовый URL:', 'All requests are sent to the base URL:')}
            </p>
            <Pre>{'https://id.cotton.app/api/v1'}</Pre>
            <div style={{ marginTop: 12, display: 'grid', gap: 8 }}>
              {[
                { method: 'POST', path: '/auth/signup', desc: t('Создание аккаунта', 'Create account') },
                { method: 'POST', path: '/auth/login', desc: t('Вход по паролю', 'Password login') },
                { method: 'POST', path: '/auth/logout', desc: t('Выход', 'Logout') },
                { method: 'GET', path: '/auth/session', desc: t('Текущая сессия', 'Current session') },
                { method: 'POST', path: '/oauth/login/accept', desc: t('Принять login challenge', 'Accept login challenge') },
                { method: 'GET', path: '/oauth/consent', desc: t('Запрос consent', 'Consent request') },
              ].map(({ method, path, desc }) => (
                <div key={path} className="row gap-3" style={{ alignItems: 'center', background: 'var(--surface-2)', borderRadius: 8, padding: '8px 12px', fontSize: 13.5 }}>
                  <span style={{
                    fontWeight: 600, fontSize: 11.5, letterSpacing: '.05em',
                    color: method === 'GET' ? 'var(--good)' : 'var(--accent)',
                  }}>{method}</span>
                  <Code>{path}</Code>
                  <span className="spacer" />
                  <span className="faint">{desc}</span>
                </div>
              ))}
            </div>
          </Section>

          <Section title={t('Пример запроса', 'Example Request')}>
            <p className="muted" style={{ fontSize: 14.5, lineHeight: 1.6 }}>
              {t('Авторизация через Authorization Code Flow:', 'Authorization via the Authorization Code Flow:')}
            </p>
            <Pre>{`GET /authorize?
  client_id=cotton_a1b2
  &response_type=code
  &scope=openid+profile+email
  &redirect_uri=https://example.com/cb
  &state=xyz789`}</Pre>
            <p className="muted" style={{ fontSize: 14.5, marginTop: 16, lineHeight: 1.6 }}>
              {t('После подтверждения пользователем, сервер ответит редиректом на redirect_uri с code и state.', 'After the user approves, the server redirects to the redirect_uri with a code and state.')}
            </p>
            <Pre>{`POST /token\nContent-Type: application/x-www-form-urlencoded\n\ngrant_type=authorization_code\n&code=AUTH_CODE\n&redirect_uri=https://example.com/cb\n&client_id=cotton_a1b2\n&client_secret=SECRET`}</Pre>
          </Section>

          <Section title={t('Scopes', 'Scopes')}>
            <div style={{ display: 'grid', gap: 8 }}>
              {[
                { scope: 'openid', desc: t('Обязательный scope для OIDC', 'Required scope for OIDC') },
                { scope: 'profile', desc: t('Имя, username и аватар', 'Name, username and avatar') },
                { scope: 'email', desc: t('Email пользователя', 'User email') },
              ].map(({ scope, desc }) => (
                <div key={scope} className="row gap-3" style={{ alignItems: 'center', background: 'var(--surface-2)', borderRadius: 8, padding: '8px 12px', fontSize: 13.5 }}>
                  <Code>{scope}</Code>
                  <span className="faint">{desc}</span>
                </div>
              ))}
            </div>
          </Section>

          <Section title={t('Поддержка', 'Support')}>
            <p className="muted" style={{ fontSize: 14.5, lineHeight: 1.6 }}>
              {t('По вопросам интеграции пишите на support@cotton.app или откройте issue на GitHub.', 'For integration questions, email support@cotton.app or open an issue on GitHub.')}
            </p>
          </Section>
        </div>
      </div>
    </div>
  );
}
