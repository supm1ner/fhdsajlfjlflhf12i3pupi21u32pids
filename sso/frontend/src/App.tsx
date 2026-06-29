/* ============================================================================
 * App.tsx — root: blob background + router.
 * The animated background is rendered once, behind the routed screens. CSRF is
 * established eagerly so the first mutating request already has a token cookie.
 * ==========================================================================*/

import { useEffect } from 'react';
import { Route, Routes } from 'react-router-dom';
import { BlobBackground } from './components/BlobBackground';
import { api } from './lib/api';
import { Account } from './routes/Account';
import {
  Admin,
  AdminJournalScreen,
  AdminOverviewScreen,
  AdminSessionsScreen,
  AdminServicesScreen,
  AdminSettingsScreen,
  AdminUserScreen,
  AdminUsersScreen,
} from './routes/Admin';
import { Consent } from './routes/Consent';
import { Docs } from './routes/Docs';
import { Forgot } from './routes/Forgot';
import { Landing } from './routes/Landing';
import { Login } from './routes/Login';
import { NotFound } from './routes/NotFound';
import { Passkeys } from './routes/Passkeys';
import { Reset } from './routes/Reset';
import { Signup } from './routes/Signup';

export function App(): JSX.Element {
  useEffect(() => {
    // Warm the CSRF cookie/token. Best-effort: if the backend isn't up yet the
    // token is lazily (re)fetched on the first mutating request anyway.
    api.csrf().catch(() => {
      /* ignore — fetched on demand by the api client */
    });
  }, []);

  return (
    <>
      <BlobBackground />
      <Routes>
        <Route path="/" element={<Landing />} />
        <Route path="/login" element={<Login />} />
        <Route path="/signup" element={<Signup />} />
        <Route path="/forgot" element={<Forgot />} />
        <Route path="/reset" element={<Reset />} />
        <Route path="/passkeys" element={<Passkeys />} />
        <Route path="/account" element={<Account />} />
        <Route path="/admin" element={<Admin />}>
          <Route index element={<AdminOverviewScreen />} />
          <Route path="users" element={<AdminUsersScreen />} />
          <Route path="users/:id" element={<AdminUserScreen />} />
          <Route path="sessions" element={<AdminSessionsScreen />} />
          <Route path="journal" element={<AdminJournalScreen />} />
          <Route path="services" element={<AdminServicesScreen />} />
          <Route path="settings" element={<AdminSettingsScreen />} />
        </Route>
        <Route path="/docs" element={<Docs />} />
        <Route path="/consent" element={<Consent />} />
        <Route path="*" element={<NotFound />} />
      </Routes>
    </>
  );
}
