import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';

import '../state/app_state.dart';
import '../sunrise/sso.dart';
import '../theme/glass_theme.dart';
import '../widgets/glass.dart';

class LoginScreen extends StatefulWidget {
  const LoginScreen({super.key, required this.state});
  final AppState state;

  @override
  State<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends State<LoginScreen> {
  final _login = TextEditingController();
  final _password = TextEditingController();
  final _sso = Sso(const SsoConfig());

  @override
  void dispose() {
    _login.dispose();
    _password.dispose();
    super.dispose();
  }

  bool get _busy => widget.state.phase == Phase.connecting;

  Future<void> _signIn() async {
    await widget.state.loginBasic(_login.text.trim(), _password.text);
  }

  Future<void> _register() async {
    await widget.state.register(_login.text.trim(), _password.text);
  }

  Future<void> _signInSso() async {
    final pkce = Pkce.generate();
    final url = _sso.authorizationUrl(pkce);
    await launchUrl(Uri.parse(url), mode: LaunchMode.externalApplication);
    if (!mounted) return;
    final code = await _askCode();
    if (code == null || code.isEmpty) return;
    try {
      final idToken = await _sso.exchangeCode(code, pkce.verifier);
      await widget.state.loginOidc(idToken);
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('$e')));
    }
  }

  Future<String?> _askCode() {
    final ctrl = TextEditingController();
    return showDialog<String>(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: Palette.bg1,
        title: const Text('Complete SSO sign-in'),
        content: Column(mainAxisSize: MainAxisSize.min, children: [
          const Text('After signing in, paste the authorization code from the redirect URL.',
              style: TextStyle(color: Palette.textSecondary, fontSize: 13)),
          const SizedBox(height: 12),
          GlassField(controller: ctrl, hint: 'Authorization code'),
        ]),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx), child: const Text('Cancel')),
          TextButton(onPressed: () => Navigator.pop(ctx, ctrl.text.trim()), child: const Text('Continue')),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: DecoratedBox(
        decoration: const BoxDecoration(gradient: Palette.backdrop),
        child: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 380),
            child: Padding(
              padding: const EdgeInsets.all(24),
              child: GlassPanel(
                padding: const EdgeInsets.all(28),
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    const Text('☀️', style: TextStyle(fontSize: 44)),
                    const SizedBox(height: 8),
                    const Text('Sunrise',
                        style: TextStyle(fontSize: 24, fontWeight: FontWeight.w700, color: Palette.textPrimary)),
                    const SizedBox(height: 4),
                    const Text('Welcome back', style: TextStyle(color: Palette.textSecondary, fontSize: 13)),
                    const SizedBox(height: 20),
                    if (widget.state.error != null) ...[
                      Text(widget.state.error!, style: const TextStyle(color: Palette.danger, fontSize: 12)),
                      const SizedBox(height: 12),
                    ],
                    GlassField(controller: _login, hint: 'Login', icon: Icons.person_outline),
                    const SizedBox(height: 12),
                    GlassField(
                        controller: _password,
                        hint: 'Password',
                        icon: Icons.lock_outline,
                        obscure: true,
                        onSubmitted: (_) => _signIn()),
                    const SizedBox(height: 18),
                    GlassButton(label: 'Sign In', primary: true, expand: true, loading: _busy, onTap: _signIn),
                    const SizedBox(height: 8),
                    GlassButton(label: 'Register New Account', expand: true, loading: _busy, onTap: _register),
                    const SizedBox(height: 12),
                    Row(children: const [
                      Expanded(child: Divider(color: Palette.glassBorder)),
                      Padding(
                          padding: EdgeInsets.symmetric(horizontal: 10),
                          child: Text('or', style: TextStyle(color: Palette.textTertiary, fontSize: 12))),
                      Expanded(child: Divider(color: Palette.glassBorder)),
                    ]),
                    const SizedBox(height: 12),
                    GlassButton(
                        label: 'Sign in with SSO', icon: Icons.shield_outlined, expand: true, onTap: _signInSso),
                  ],
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}
