import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactNode } from 'react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  Admin,
  AdminJournalScreen,
  AdminOverviewScreen,
  AdminUserScreen,
  AdminUsersScreen,
} from './Admin';
import { AppProvider } from '../i18n/context';
import { I18N } from '../i18n/dictionary';
import * as apiModule from '../lib/api';
import type { AdminAuditResponse, AdminUserDetail, AdminUsersResponse, User } from '../lib/api';

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

const PLAIN_USER: User = {
  ...OWNER,
  id: 'me-user',
  username: 'joe',
  displayName: 'Joe User',
  role: 'user',
};

const USERS_PAGE: AdminUsersResponse = {
  users: [
    {
      id: 'u-alex',
      displayName: 'Alex Renn',
      username: 'alex',
      email: 'alex@cotton.id',
      status: 'active',
      role: 'admin',
      services: 8,
      createdAt: '2024-02-12T10:00:00Z',
    },
    {
      id: 'u-tom',
      displayName: 'Tom Wilder',
      username: 'twild',
      email: 'tom@wilder.dev',
      status: 'suspended',
      role: 'user',
      services: 2,
      createdAt: '2024-06-14T10:00:00Z',
    },
  ],
  total: 2,
  page: 1,
  pageSize: 20,
};

const USER_DETAIL: AdminUserDetail = {
  id: 'u-tom',
  displayName: 'Tom Wilder',
  username: 'twild',
  email: 'tom@wilder.dev',
  emailVerified: true,
  status: 'active',
  role: 'user',
  about: 'Builder.',
  location: 'Austin, US',
  createdAt: '2024-06-14T10:00:00Z',
  updatedAt: '2025-01-01T10:00:00Z',
  counts: { sessions: 1, services: 2, logins: 94 },
  sessions: [
    {
      id: 's1',
      userAgent: 'Mozilla/5.0 (Macintosh) Safari/605',
      ip: '203.0.113.5',
      createdAt: '2025-01-01T09:00:00Z',
      expiresAt: '2025-02-01T09:00:00Z',
    },
  ],
  connections: [
    {
      client: 'app-vault',
      clientName: 'Vault',
      grantedScopes: ['openid'],
      grantedAt: '2024-12-01T09:00:00Z',
    },
  ],
  recentActivity: [
    { id: 'a1', ts: '2025-01-01T09:00:00Z', action: 'login.ok', actorLabel: 'twild' },
  ],
};

const AUDIT_PAGE: AdminAuditResponse = {
  entries: [
    {
      id: 'e1',
      ts: '2025-01-02T10:00:00Z',
      actorLabel: 'alex',
      action: 'user.suspend',
      targetType: 'user',
      targetId: 'u-tom',
      ip: '203.0.113.9',
    },
    {
      id: 'e2',
      ts: '2025-01-02T09:00:00Z',
      action: 'login.fail',
      ip: '198.51.100.2',
    },
  ],
  total: 2,
  page: 1,
  pageSize: 20,
};

/** Render the admin route tree (shell + nested screens) at `route`. */
function renderAdmin(route: string): void {
  const Wrapper = ({ children }: { children: ReactNode }): JSX.Element => (
    <MemoryRouter initialEntries={[route]}>
      <AppProvider>{children}</AppProvider>
    </MemoryRouter>
  );
  render(
    <Routes>
      <Route path="/login" element={<div>LOGIN PAGE</div>} />
      <Route path="/" element={<div>HOME PAGE</div>} />
      <Route path="/admin" element={<Admin />}>
        <Route index element={<AdminOverviewScreen />} />
        <Route path="users" element={<AdminUsersScreen />} />
        <Route path="users/:id" element={<AdminUserScreen />} />
        <Route path="journal" element={<AdminJournalScreen />} />
      </Route>
    </Routes>,
    { wrapper: Wrapper },
  );
}

describe('Admin console', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe('role gate', () => {
    it('renders a not-authorized note for a signed-in non-admin', async () => {
      vi.spyOn(apiModule.api, 'session').mockResolvedValue({ user: PLAIN_USER });

      renderAdmin('/admin');

      expect(await screen.findByText(t.adm_not_authorized_title)).toBeInTheDocument();
      // The console chrome (search bar) must NOT render for a non-admin.
      expect(screen.queryByPlaceholderText(t.adm_search)).toBeNull();
    });

    it('redirects to /login when there is no session', async () => {
      vi.spyOn(apiModule.api, 'session').mockRejectedValue(
        new apiModule.ApiError(401, { title: 'Unauthorized', status: 401 }),
      );

      renderAdmin('/admin');

      expect(await screen.findByText('LOGIN PAGE')).toBeInTheDocument();
    });

    it('admits an admin/owner and shows the console shell', async () => {
      vi.spyOn(apiModule.api, 'session').mockResolvedValue({ user: OWNER });
      vi.spyOn(apiModule.adminApi, 'overview').mockResolvedValue({
        stats: { totalUsers: 12, activeToday: 3, newThisWeek: 2, services: 5 },
        signups: [{ date: '2025-01-01', count: 4 }],
        recentSignups: [],
        recentActivity: [],
      });

      renderAdmin('/admin');

      // The search bar (shell chrome) appears once authorized.
      expect(await screen.findByPlaceholderText(t.adm_search)).toBeInTheDocument();
      // Stat card labels render from the mocked overview.
      expect(await screen.findByText(t.adm_stat_users)).toBeInTheDocument();
    });
  });

  describe('users table', () => {
    beforeEach(() => {
      vi.spyOn(apiModule.api, 'session').mockResolvedValue({ user: OWNER });
    });

    it('renders rows from a mocked API and filters by status', async () => {
      const users = vi.spyOn(apiModule.adminApi, 'users').mockResolvedValue(USERS_PAGE);

      renderAdmin('/admin/users');

      // Both seeded users render.
      expect(await screen.findByText('Alex Renn')).toBeInTheDocument();
      expect(screen.getByText('Tom Wilder')).toBeInTheDocument();

      // Initial load: no status/role constraint.
      await waitFor(() => expect(users).toHaveBeenCalled());
      expect(users.mock.calls[0]![0]).toMatchObject({ page: 1, pageSize: 20 });
      expect(users.mock.calls[0]![0]).not.toHaveProperty('status');

      // Clicking the "Suspended" status filter refetches with status=suspended.
      await userEvent.click(screen.getByRole('tab', { name: t.adm_filter_suspended }));
      await waitFor(() =>
        expect(users.mock.calls.some((c) => c[0]?.status === 'suspended')).toBe(true),
      );
    });

    it('opens the per-user detail when a row is clicked', async () => {
      vi.spyOn(apiModule.adminApi, 'users').mockResolvedValue(USERS_PAGE);
      const detail = vi.spyOn(apiModule.adminApi, 'user').mockResolvedValue(USER_DETAIL);

      renderAdmin('/admin/users');

      await userEvent.click(await screen.findByText('Tom Wilder'));

      await waitFor(() => expect(detail).toHaveBeenCalledWith('u-tom', expect.anything()));
      // The detail action panel renders.
      expect(await screen.findByText(t.adm_card_actions)).toBeInTheDocument();
    });
  });

  describe('destructive actions', () => {
    beforeEach(() => {
      vi.spyOn(apiModule.api, 'session').mockResolvedValue({ user: OWNER });
      vi.spyOn(apiModule.adminApi, 'user').mockResolvedValue(USER_DETAIL);
    });

    it('does not suspend until the confirm is accepted', async () => {
      const suspend = vi.spyOn(apiModule.adminApi, 'suspendUser').mockResolvedValue();
      const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false);

      renderAdmin('/admin/users/u-tom');

      const btn = await screen.findByRole('button', { name: new RegExp(t.adm_act_suspend, 'i') });

      // Cancelled confirm → no call.
      await userEvent.click(btn);
      expect(confirm).toHaveBeenCalledWith(t.adm_confirm_suspend);
      expect(suspend).not.toHaveBeenCalled();

      // Accepted confirm → the endpoint is hit.
      confirm.mockReturnValue(true);
      await userEvent.click(btn);
      await waitFor(() => expect(suspend).toHaveBeenCalledWith('u-tom'));
    });

    it('hides owner-only actions (delete / change role) for a non-owner admin', async () => {
      vi.spyOn(apiModule.api, 'session').mockResolvedValue({
        user: { ...OWNER, id: 'me-admin', role: 'admin' },
      });

      renderAdmin('/admin/users/u-tom');

      await screen.findByText(t.adm_card_actions);
      expect(screen.queryByRole('button', { name: new RegExp(t.adm_act_delete, 'i') })).toBeNull();
      expect(screen.queryByRole('button', { name: new RegExp(t.adm_act_role, 'i') })).toBeNull();
    });
  });

  describe('journal', () => {
    beforeEach(() => {
      vi.spyOn(apiModule.api, 'session').mockResolvedValue({ user: OWNER });
    });

    it('renders audit rows from a mocked API and applies a filter', async () => {
      const audit = vi.spyOn(apiModule.adminApi, 'audit').mockResolvedValue(AUDIT_PAGE);

      renderAdmin('/admin/journal');

      // Rows render: action keys + actor + ip.
      expect(await screen.findByText('user.suspend')).toBeInTheDocument();
      expect(screen.getByText('login.fail')).toBeInTheDocument();
      expect(screen.getByText('alex')).toBeInTheDocument();
      // System actor fallback for the actor-less row.
      expect(screen.getByText(t.adm_system_actor)).toBeInTheDocument();

      // Typing an action filter + Apply refetches with that action. Exclude the
      // shell's search box (it carries the search placeholder); the first of the
      // remaining textboxes is the "action" filter field.
      const actionInput = screen
        .getAllByRole('textbox')
        .find((el) => el.getAttribute('placeholder') !== t.adm_search)!;
      await userEvent.type(actionInput, 'login.fail');
      await userEvent.click(screen.getByRole('button', { name: t.adm_journal_filter_apply }));

      await waitFor(() =>
        expect(audit.mock.calls.some((c) => c[0]?.action === 'login.fail')).toBe(true),
      );
    });
  });
});
