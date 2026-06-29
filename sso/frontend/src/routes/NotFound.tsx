/* NotFound.tsx — catch-all 404 within the SPA. */

import { useNavigate } from 'react-router-dom';
import { AuthChrome } from '../components/AuthChrome';
import { Button } from '../components/Button';
import { Logo } from '../components/Logo';

export function NotFound(): JSX.Element {
  const navigate = useNavigate();
  return (
    <AuthChrome>
      <div
        className="glass rise center"
        style={{
          width: '100%',
          maxWidth: 420,
          borderRadius: 'var(--r-xl)',
          padding: '48px 38px',
          gap: 22,
          textAlign: 'center',
        }}
      >
        <Logo size={30} />
        <h1 style={{ fontFamily: 'var(--serif)', fontWeight: 400, fontSize: 64, lineHeight: 1 }}>
          404
        </h1>
        <Button size="lg" full onClick={() => navigate('/')}>
          cotton-id
        </Button>
      </div>
    </AuthChrome>
  );
}
