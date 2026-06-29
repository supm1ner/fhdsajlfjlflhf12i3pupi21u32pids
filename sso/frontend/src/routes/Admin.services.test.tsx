import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactNode } from 'react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Admin, AdminServicesScreen } from './Admin';
import { AppProvider } from '../i18n/context';
import { I18N } from '../i18n/dictionary';
import * as apiModule from '../lib/api';
import type { Client, CreatedClient, User } from '../lib/api';

const t = I18N.ru; // RU is the default locale.

/* ----------------------------- fixtures ----------------------------- */

const OWNER: User = {
  id: 'me-owner',
  email: 'owner@cotton.id',
  emailVerified: true,
  username: 'owner',
  displayName: 'Olga Owner',
  role: 'owner',
};

const PLAIN_USER: User = { ...OWNER, id: 'me-user', role: 'user' };

const CLIENTS: Client[] = [
  {
    id: 'app-vault',
    name: 'Vault',
    type: 'confidential',
    redirectUris: ['https://vault.example.com/callback'],
    scopes: ['openid', 'profile', 'email'],
    grantTypes: ['authorization_code', 'refresh_token'],
    responseTypes: ['code'],
    createdAt: '2025-01-01T10:00:00Z',
  },
  {
    id: 'app-studio',
    name: 'Studio',
    type: 'public',
    redirectUris: ['https://studio.example.com/cb'],
    scopes: ['openid'],
    grantTypes: ['authorization_code'],
    responseTypes: ['code'],
  },
];

/** Render the admin route tree with the Services screen mounted at /admin/services. */
function renderServices(): void {
  const Wrapper = ({ children }: { children: ReactNode }): JSX.Element => (
    <MemoryRouter initialEntries={['/admin/services']}>
      <AppProvider>{children}</AppProvider>
    </MemoryRouter>
  );
  render(
    <Routes>
      <Route path="/login" element={<div>LOGIN PAGE</div>} />
      <Route path="/admin" element={<Admin />}>
        <Route path="services" element={<AdminServicesScreen />} />
      </Route>
    </Routes>,
    { wrapper: Wrapper },
  );
}

describe('Admin Services tab', () => {
  beforeEach(() => {
    vi.spyOn(apiModule.api, 'session').mockResolvedValue({ user: OWNER });
    // Default: each client reports a consent count (lazily hydrated per row).
    vi.spyOn(apiModule.adminServicesApi, 'consentCount').mockResolvedValue(3);
  });
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders registered clients from a mocked API', async () => {
    vi.spyOn(apiModule.adminServicesApi, 'list').mockResolvedValue(CLIENTS);

    renderServices();

    // Both clients render (name + id), with their type badges.
    expect(await screen.findByText('Vault')).toBeInTheDocument();
    expect(screen.getByText('Studio')).toBeInTheDocument();
    expect(screen.getByText('app-vault')).toBeInTheDocument();
    expect(screen.getByText(t.adm_services_type_confidential)).toBeInTheDocument();
    expect(screen.getByText(t.adm_services_type_public)).toBeInTheDocument();
    // A redirect URI + a scope chip surface in the table.
    expect(screen.getByText('https://vault.example.com/callback')).toBeInTheDocument();
    expect(screen.getAllByText('openid').length).toBeGreaterThan(0);

    // Consent counts hydrate per row (best-effort).
    await waitFor(() => expect(screen.getAllByText('3').length).toBeGreaterThan(0));
  });

  it('renders the empty state when there are no clients', async () => {
    vi.spyOn(apiModule.adminServicesApi, 'list').mockResolvedValue([]);

    renderServices();

    expect(await screen.findByText(t.adm_services_empty)).toBeInTheDocument();
  });

  it('creates a confidential client and shows the secret exactly once', async () => {
    vi.spyOn(apiModule.adminServicesApi, 'list').mockResolvedValue(CLIENTS);
    const created: CreatedClient = {
      client: {
        id: 'app-new',
        name: 'New App',
        type: 'confidential',
        redirectUris: ['https://new.example.com/callback'],
        scopes: ['openid'],
        grantTypes: ['authorization_code'],
        responseTypes: ['code'],
      },
      secret: 'sek_one_time_xyz',
    };
    const create = vi.spyOn(apiModule.adminServicesApi, 'create').mockResolvedValue(created);

    renderServices();

    // Open the create form.
    await userEvent.click(await screen.findByRole('button', { name: t.adm_services_register }));

    // Fill name + a redirect URI, pick confidential, submit.
    await userEvent.type(screen.getByLabelText(t.adm_services_field_name), 'New App');
    const redirectArea = screen.getByPlaceholderText(t.adm_services_field_redirects_ph);
    await userEvent.type(redirectArea, 'https://new.example.com/callback');
    await userEvent.click(screen.getByRole('tab', { name: t.adm_services_type_confidential }));

    await userEvent.click(screen.getByRole('button', { name: t.adm_services_create_submit }));

    // The create endpoint is hit with the typed input.
    await waitFor(() => expect(create).toHaveBeenCalledTimes(1));
    expect(create.mock.calls[0]![0]).toMatchObject({
      name: 'New App',
      type: 'confidential',
      redirectUris: ['https://new.example.com/callback'],
    });

    // The one-time secret panel surfaces the secret.
    expect(await screen.findByText('sek_one_time_xyz')).toBeInTheDocument();
    expect(screen.getByText(t.adm_services_secret_title)).toBeInTheDocument();

    // Dismissing the panel discards the secret (it is never re-served).
    await userEvent.click(screen.getByRole('button', { name: t.adm_services_secret_done }));
    await waitFor(() => expect(screen.queryByText('sek_one_time_xyz')).toBeNull());
  });

  it('deletes a client only after the confirm is accepted', async () => {
    vi.spyOn(apiModule.adminServicesApi, 'list').mockResolvedValue(CLIENTS);
    const del = vi.spyOn(apiModule.adminServicesApi, 'delete').mockResolvedValue();
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false);

    renderServices();

    await screen.findByText('Vault');
    const deleteButtons = screen.getAllByRole('button', {
      name: new RegExp(`^${t.adm_services_delete}$`, 'i'),
    });

    // Cancelled confirm → no call.
    await userEvent.click(deleteButtons[0]!);
    expect(confirm).toHaveBeenCalledWith(t.adm_services_delete_confirm);
    expect(del).not.toHaveBeenCalled();

    // Accepted confirm → the endpoint is hit for the first client.
    confirm.mockReturnValue(true);
    await userEvent.click(deleteButtons[0]!);
    await waitFor(() => expect(del).toHaveBeenCalledWith('app-vault'));
  });

  it("revokes a client's grants behind a confirm", async () => {
    vi.spyOn(apiModule.adminServicesApi, 'list').mockResolvedValue(CLIENTS);
    const revoke = vi.spyOn(apiModule.adminServicesApi, 'revokeConsents').mockResolvedValue();
    vi.spyOn(window, 'confirm').mockReturnValue(true);

    renderServices();

    await screen.findByText('Vault');
    const revokeButtons = screen.getAllByRole('button', { name: t.adm_services_revoke });
    await userEvent.click(revokeButtons[0]!);

    await waitFor(() => expect(revoke).toHaveBeenCalledWith('app-vault'));
  });

  describe('role gate (shell)', () => {
    it('blocks a non-admin from the Services tab', async () => {
      vi.spyOn(apiModule.api, 'session').mockResolvedValue({ user: PLAIN_USER });
      const list = vi.spyOn(apiModule.adminServicesApi, 'list').mockResolvedValue(CLIENTS);

      renderServices();

      expect(await screen.findByText(t.adm_not_authorized_title)).toBeInTheDocument();
      // The Services screen must not have fetched anything for a non-admin.
      expect(list).not.toHaveBeenCalled();
    });
  });
});
