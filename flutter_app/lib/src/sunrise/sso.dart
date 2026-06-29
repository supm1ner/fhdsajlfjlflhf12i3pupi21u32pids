import 'dart:convert';
import 'dart:io';
import 'dart:math';

import 'package:crypto/crypto.dart';

/// OpenID Connect (Authorization Code + PKCE) helper for the SSO provider
/// (Ory Hydra fronted by cotton-id). Public client: no secret, PKCE only.
///
/// Full native redirect capture requires per-platform deep-link/custom-scheme setup
/// (e.g. flutter_web_auth_2). For a first cut the UI can open [authorizationUrl] in the
/// system browser and let the user paste the returned `code`, which [exchangeCode]
/// trades for an ID token.
class SsoConfig {
  const SsoConfig({
    this.issuer = 'http://localhost:4444/',
    this.clientId = 'sunrise-messenger',
    this.redirectUri = 'http://localhost:5173/',
    this.scopes = 'openid profile email',
  });

  final String issuer;
  final String clientId;
  final String redirectUri;
  final String scopes;
}

class Pkce {
  Pkce(this.verifier, this.challenge, this.state);
  final String verifier;
  final String challenge;
  final String state;

  static String _b64url(List<int> bytes) =>
      base64Url.encode(bytes).replaceAll('=', '');

  static String _randomString(int len) {
    final rnd = Random.secure();
    return _b64url(List<int>.generate(len, (_) => rnd.nextInt(256)));
  }

  factory Pkce.generate() {
    final verifier = _randomString(32);
    final challenge = _b64url(sha256.convert(ascii.encode(verifier)).bytes);
    final state = _randomString(16);
    return Pkce(verifier, challenge, state);
  }
}

class Sso {
  Sso(this.config);
  final SsoConfig config;

  String authorizationUrl(Pkce pkce) {
    final q = {
      'response_type': 'code',
      'client_id': config.clientId,
      'redirect_uri': config.redirectUri,
      'scope': config.scopes,
      'state': pkce.state,
      'code_challenge': pkce.challenge,
      'code_challenge_method': 'S256',
    };
    final query = q.entries.map((e) => '${e.key}=${Uri.encodeComponent(e.value)}').join('&');
    return '${config.issuer}oauth2/auth?$query';
  }

  /// Exchanges an authorization code for tokens; returns the ID token.
  Future<String> exchangeCode(String code, String verifier) async {
    final uri = Uri.parse('${config.issuer}oauth2/token');
    final body = {
      'grant_type': 'authorization_code',
      'code': code,
      'redirect_uri': config.redirectUri,
      'client_id': config.clientId,
      'code_verifier': verifier,
    };
    final client = HttpClient();
    try {
      final req = await client.postUrl(uri);
      req.headers.set(HttpHeaders.contentTypeHeader, 'application/x-www-form-urlencoded');
      req.write(body.entries.map((e) => '${e.key}=${Uri.encodeComponent(e.value)}').join('&'));
      final resp = await req.close();
      final text = await resp.transform(utf8.decoder).join();
      if (resp.statusCode < 200 || resp.statusCode >= 300) {
        throw Exception('Token exchange failed: ${resp.statusCode} $text');
      }
      final json = jsonDecode(text) as Map<String, dynamic>;
      final idToken = json['id_token'] as String?;
      if (idToken == null) throw Exception('No id_token in response');
      return idToken;
    } finally {
      client.close();
    }
  }
}
