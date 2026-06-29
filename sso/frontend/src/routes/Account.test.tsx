import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Account } from './Account';
import { I18N } from '../i18n/dictionary';
import * as apiModule from '../lib/api';
import type {
  Account as AccountPayload,
  AccountConnection,
  AccountSession,
} from '../lib/api';
import { renderWithProviders } from '../test/render';

const t = I18N.ru; // RU default.

const FAKE_ACCOUNT: AccountPayload = {
  id: 'u1',
  email: 'alex@cotton.id',
  emailVerified: true,
  username: 'alex',
  displayName: 'Alex Cotton',
  role: 'user',
  status: 'active',
  about: 'Builder of things.',
  location: 'Almaty, KZ',
  hasPassword: true,
  createdAt: '2025-01-10T10:00:00Z',
  updatedAt: '2026-05-01T10:00:00Z',
  preferences: { theme: 'dark', lang: 'ru', loginNotifications: true },
  counts: { sessions: 2, passkeys: 1, connections: 1 },
};

const FAKE_SESSIONS: AccountSession[] = [
  {
    id: 'sess-current',
    userAgent: 'Mozilla/5.0 (Macintosh) Safari/605',
    ip: '203.0.113.1',
    createdAt: '2026-05-01T09:00:00Z',
    lastSeenAt: '2026-06-01T09:00:00Z',
    expiresAt: '2026-07-01T09:00:00Z',
    current: true,
  },
  {
    id: 'sess-other',
    userAgent: 'Mozilla/5.0 (Windows NT 10.0) Chrome/120',
    ip: '198.51.100.7',
    createdAt: '2026-04-20T09:00:00Z',
    lastSeenAt: '2026-05-20T09:00:00Z',
    expiresAt: '2026-06-20T09:00:00Z',
    current: false,
  },
];

const FAKE_CONNECTIONS: AccountConnection[] = [
  {
    client: 'app-photos',
    clientName: 'Cotton Photos',
    grantedScopes: ['openid', 'profile'],
    grantedAt: '2026-03-01T09:00:00Z',
  },
];

/** Stub every network call the screen + its sections make on mount. */
function stubLoad(overrides?: { account?: AccountPayload }): void {
  vi.spyOn(apiModule.accountApi, 'get').mockResolvedValue(overrides?.account ?? FAKE_ACCOUNT);
  vi.spyOn(apiModule.accountApi, 'sessions').mockResolvedValue({ sessions: FAKE_SESSIONS });
  vi.spyOn(apiModule.accountApi, 'connections').mockResolvedValue({ connections: FAKE_CONNECTIONS });
  vi.spyOn(apiModule.passkeyApi, 'list').mockResolvedValue({ passkeys: [] });
}

describe('Account screen', () => {
  beforeEach(() => {
    stubLoad();
    // WebAuthn absent by default — the passkey "add" button is then hidden.
    delete (window as unknown as { PublicKeyCredential?: unknown }).PublicKeyCredential;
  });
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders the profile header and sections from a mocked /account', async () => {
    renderWithProviders(<Account />, { route: '/account' });

    // Profile header identity.
    expect(await screen.findByRole('heading', { name: 'Alex Cotton' })).toBeInTheDocument();
    expect(screen.getByText(/alex@cotton\.id/)).toBeInTheDocument();
    expect(screen.getByText('@alex')).toBeInTheDocument();

    // The four section tabs are present.
    expect(screen.getByRole('tab', { name: t.acc_nav_profile })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: t.acc_nav_security })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: t.acc_nav_services })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: t.acc_nav_settings })).toBeInTheDocument();

    // Profile tab editable fields are seeded from the payload.
    expect(screen.getByDisplayValue('Builder of things.')).toBeInTheDocument();
    expect(screen.getByDisplayValue('Almaty, KZ')).toBeInTheDocument();
  });

  it('redirects to /login when the account load returns 401', async () => {
    vi.spyOn(apiModule.accountApi, 'get').mockRejectedValue(
      new apiModule.ApiError(401, { title: 'Unauthorized', status: 401 }),
    );

    renderWithProviders(<Account />, { route: '/account' });

    // The screen renders neither the heading nor an error once it bounces.
    await waitFor(() => {
      expect(screen.queryByRole('heading', { name: 'Alex Cotton' })).toBeNull();
    });
  });

  it('submits a profile edit as a PATCH', async () => {
    const updateProfile = vi
      .spyOn(apiModule.accountApi, 'updateProfile')
      .mockResolvedValue({ ...FAKE_ACCOUNT, displayName: 'Alex Renamed' });

    renderWithProviders(<Account />, { route: '/account' });

    const nameInput = await screen.findByDisplayValue('Alex Cotton');
    await userEvent.clear(nameInput);
    await userEvent.type(nameInput, 'Alex Renamed');

    await userEvent.click(screen.getByRole('button', { name: new RegExp(t.acc_save, 'i') }));

    await waitFor(() => expect(updateProfile).toHaveBeenCalledTimes(1));
    expect(updateProfile).toHaveBeenCalledWith({
      displayName: 'Alex Renamed',
      about: 'Builder of things.',
      location: 'Almaty, KZ',
    });
    expect(await screen.findByText(t.acc_saved)).toBeInTheDocument();
  });

  it('lists active sessions and revokes a non-current one', async () => {
    const revokeSession = vi.spyOn(apiModule.accountApi, 'revokeSession').mockResolvedValue();
    // After revoking, the list refetch returns only the current session.
    vi.spyOn(apiModule.accountApi, 'sessions')
      .mockResolvedValueOnce({ sessions: FAKE_SESSIONS })
      .mockResolvedValue({ sessions: [FAKE_SESSIONS[0]!] });
    vi.spyOn(window, 'confirm').mockReturnValue(true);

    renderWithProviders(<Account />, { route: '/account' });

    // Switch to the Security tab where sessions render.
    await userEvent.click(await screen.findByRole('tab', { name: t.acc_nav_security }));

    // Both devices render; the current one is badged, the other has a revoke btn.
    expect(await screen.findByText('Safari · macOS')).toBeInTheDocument();
    expect(screen.getByText('Chrome · Windows')).toBeInTheDocument();
    expect(screen.getByText(t.acc_this_device)).toBeInTheDocument();

    const revokeBtn = screen.getByRole('button', { name: new RegExp(`^${t.acc_revoke}$`, 'i') });
    await userEvent.click(revokeBtn);

    await waitFor(() => expect(revokeSession).toHaveBeenCalledWith('sess-other'));
    expect(await screen.findByText(t.acc_session_revoked)).toBeInTheDocument();
  });

  it('renders connected services and revokes one', async () => {
    const revokeConnection = vi
      .spyOn(apiModule.accountApi, 'revokeConnection')
      .mockResolvedValue();
    vi.spyOn(apiModule.accountApi, 'connections')
      .mockResolvedValueOnce({ connections: FAKE_CONNECTIONS })
      .mockResolvedValue({ connections: [] });
    vi.spyOn(window, 'confirm').mockReturnValue(true);

    renderWithProviders(<Account />, { route: '/account' });

    await userEvent.click(await screen.findByRole('tab', { name: t.acc_nav_services }));

    expect(await screen.findByText('Cotton Photos')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: new RegExp(t.acc_disconnect, 'i') }));

    await waitFor(() => expect(revokeConnection).toHaveBeenCalledWith('app-photos'));
    expect(await screen.findByText(t.acc_service_revoked)).toBeInTheDocument();
  });

  it('requires the password and confirmation phrase before deleting the account', async () => {
    const remove = vi.spyOn(apiModule.accountApi, 'remove').mockResolvedValue();

    renderWithProviders(<Account />, { route: '/account' });

    await userEvent.click(await screen.findByRole('tab', { name: t.acc_nav_settings }));

    // Open the danger-zone modal.
    const dangerDelete = screen.getByRole('button', { name: new RegExp(`^${t.acc_delete}$`, 'i') });
    await userEvent.click(dangerDelete);

    const dialog = await screen.findByRole('dialog');
    const confirmBtn = within(dialog).getByRole('button', {
      name: new RegExp(t.acc_delete_confirm_btn, 'i'),
    });

    // Disabled until BOTH the password and the exact phrase are supplied.
    expect(confirmBtn).toBeDisabled();

    const pwInput = within(dialog).getByLabelText(t.acc_delete_confirm_pw);
    await userEvent.type(pwInput, 'hunter2');
    expect(confirmBtn).toBeDisabled(); // phrase still missing

    const phraseInput = within(dialog).getByLabelText(t.acc_delete_confirm_phrase);
    await userEvent.type(phraseInput, t.acc_delete_confirm_word);

    await waitFor(() => expect(confirmBtn).toBeEnabled());
    expect(remove).not.toHaveBeenCalled();

    await userEvent.click(confirmBtn);
    await waitFor(() => expect(remove).toHaveBeenCalledWith({ password: 'hunter2' }));
  });
});
