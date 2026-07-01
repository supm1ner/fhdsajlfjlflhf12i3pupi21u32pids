<script>
  import { onMount } from 'svelte';
  import { appState } from '../lib/stores.svelte.js';
  import { connect, login, loginWithToken, getClient } from '../lib/tinode.js';
  import { beginLogin, completeLogin, isRedirectCallback } from '../lib/oidc.js';
  import GlassPanel from '../lib/components/GlassPanel.svelte';
  import Input from '../lib/components/Input.svelte';
  import Button from '../lib/components/Button.svelte';

  let loginVal = $state('');
  let password = $state('');
  let error = $state('');
  let loading = $state(false);

  // Establish the messenger session after any successful authentication.
  async function enterApp(displayName) {
    const c = getClient();
    appState.user = { id: c.getCurrentUserID?.() || '', name: displayName };
    appState.connected = true;
    appState.view = 'app';
  }

  // If we returned from the SSO provider, finish the OIDC flow and sign in.
  onMount(async () => {
    if (!isRedirectCallback()) return;
    loading = true;
    try {
      const idToken = await completeLogin();
      if (!idToken) return;
      await connect();
      await loginWithToken(idToken);
      await enterApp('SSO user');
    } catch (e) {
      error = e?.message || 'SSO login failed';
    } finally { loading = false; }
  });

  async function handleSsoLogin() {
    error = '';
    try {
      await beginLogin();
    } catch (e) {
      error = e?.message || 'Could not start SSO login';
    }
  }

  async function handleLogin() {
    if (!loginVal || !password) { error = 'Fill in all fields'; return; }
    error = ''; loading = true;
    try {
      await connect();
      await login(loginVal, password);
      await enterApp(loginVal);
    } catch (e) {
      error = e?.message || 'Login failed';
    } finally { loading = false; }
  }

  function setValue(field, e) {
    if (field === 'login') loginVal = e.target.value;
    else if (field === 'pass') password = e.target.value;
  }
</script>

<div class="login-page">
  <div class="bg-glow"></div>
  <GlassPanel class="login-card">
    <div class="logo">☀️</div>
    <h1 class="title">cotton Talk</h1>
    <p class="subtitle">Welcome back</p>

    {#if error}
      <div class="error">{error}</div>
    {/if}

    <Input label="Login" value={loginVal} oninput={(e) => setValue('login', e)} placeholder="Enter your login" icon="👤" onenter={handleLogin} />
    <Input label="Password" type="password" value={password} oninput={(e) => setValue('pass', e)} placeholder="Enter your password" icon="🔒" onenter={handleLogin} />

    <Button fullWidth disabled={loading} onclick={handleLogin}>{loading ? 'Connecting...' : 'Sign In'}</Button>

    <div class="divider"><span>or</span></div>

    <Button fullWidth disabled={loading} onclick={handleSsoLogin}>🔐 Sign in with SSO</Button>

    <p class="switch">
      Don't have an account? <button class="link" onclick={() => appState.view = 'register'}>Create one</button>
    </p>
  </GlassPanel>
</div>

<style>
  .login-page { height: 100%; display: flex; align-items: center; justify-content: center; position: relative; overflow: hidden; }
  .bg-glow { position: absolute; width: 600px; height: 600px; border-radius: 50%; background: radial-gradient(circle, rgba(139,92,246,0.15), transparent 70%); top: -200px; right: -200px; pointer-events: none; }
  :global(.login-card) { width: 360px; display: flex; flex-direction: column; gap: 16px; padding: 32px; animation: fadeIn 400ms ease; }
  .logo { font-size: 48px; text-align: center; }
  .title { font-size: 24px; font-weight: 600; text-align: center; }
  .subtitle { font-size: 14px; color: var(--text-secondary); text-align: center; margin-bottom: 8px; }
  .error { font-size: 13px; color: var(--danger); background: rgba(239,68,68,0.1); padding: 8px 12px; border-radius: var(--radius-sm); border: 1px solid rgba(239,68,68,0.2); }
  .divider { display: flex; align-items: center; gap: 12px; color: var(--text-secondary); font-size: 12px; }
  .divider::before, .divider::after { content: ''; flex: 1; height: 1px; background: var(--border-glass); }
  .switch { font-size: 13px; color: var(--text-secondary); text-align: center; }
  .link { background: none; border: none; color: var(--accent); cursor: pointer; font-size: 13px; font-weight: 500; }
  .link:hover { text-decoration: underline; }
</style>
