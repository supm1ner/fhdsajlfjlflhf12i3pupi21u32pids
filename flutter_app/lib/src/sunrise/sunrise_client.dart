import 'dart:async';
import 'dart:convert';

import 'package:web_socket_channel/web_socket_channel.dart';

/// Minimal client for the Sunrise (Tinode-compatible) server: WebSocket transport,
/// JSON packets, request/response correlation by id, and broadcast streams for
/// server-pushed {data}, {meta}, {pres} and {info} messages.
///
/// This implements the subset needed by the messenger UI: hi, login (basic/token/oidc),
/// sub, get (history), pub, note (typing/read). It deliberately mirrors the protocol used
/// by the JS SDK so it talks to the same backend.
class SunriseClient {
  SunriseClient({
    this.host = 'localhost:6060',
    this.apiKey = 'AQEAAAABAAD_rAp4DJh05a1HAwFT3A6K',
    this.secure = false,
    this.appName = 'SunriseFlutter/0.1',
  });

  final String host;
  final String apiKey;
  final bool secure;
  final String appName;

  WebSocketChannel? _ch;
  int _id = 0;
  final Map<String, Completer<Map<String, dynamic>>> _pending = {};

  String? userId;
  String? authToken;
  DateTime? tokenExpires;

  final _data = StreamController<Map<String, dynamic>>.broadcast();
  final _meta = StreamController<Map<String, dynamic>>.broadcast();
  final _pres = StreamController<Map<String, dynamic>>.broadcast();
  final _info = StreamController<Map<String, dynamic>>.broadcast();

  Stream<Map<String, dynamic>> get onData => _data.stream;
  Stream<Map<String, dynamic>> get onMeta => _meta.stream;
  Stream<Map<String, dynamic>> get onPres => _pres.stream;
  Stream<Map<String, dynamic>> get onInfo => _info.stream;

  bool get isConnected => _ch != null;

  String get _scheme => secure ? 'wss' : 'ws';
  String get baseHttp => '${secure ? 'https' : 'http'}://$host';
  String get wsUrl => '$_scheme://$host/v0/channels?apikey=$apiKey';

  String _nextId() => (++_id).toString();

  /// Opens the WebSocket and performs the {hi} handshake.
  Future<void> connect() async {
    if (_ch != null) return;
    final ch = WebSocketChannel.connect(Uri.parse(wsUrl));
    _ch = ch;
    ch.stream.listen(_onMessage, onError: _onError, onDone: _onDone, cancelOnError: false);
    await ch.ready;
    await _send({
      'hi': {'id': _nextId(), 'ver': '0.22', 'ua': appName},
    });
  }

  void _onMessage(dynamic raw) {
    final Map<String, dynamic> pkt;
    try {
      pkt = (jsonDecode(raw as String) as Map).cast<String, dynamic>();
    } catch (_) {
      return;
    }
    if (pkt.containsKey('ctrl')) {
      final ctrl = (pkt['ctrl'] as Map).cast<String, dynamic>();
      final id = ctrl['id'] as String?;
      if (id != null && _pending.containsKey(id)) {
        final c = _pending.remove(id)!;
        final code = (ctrl['code'] as num?)?.toInt() ?? 200;
        if (code >= 200 && code < 400) {
          c.complete(ctrl);
        } else {
          c.completeError(SunriseError(code, ctrl['text'] as String? ?? 'error'));
        }
      }
    } else if (pkt.containsKey('data')) {
      _data.add((pkt['data'] as Map).cast<String, dynamic>());
    } else if (pkt.containsKey('meta')) {
      _meta.add((pkt['meta'] as Map).cast<String, dynamic>());
    } else if (pkt.containsKey('pres')) {
      _pres.add((pkt['pres'] as Map).cast<String, dynamic>());
    } else if (pkt.containsKey('info')) {
      _info.add((pkt['info'] as Map).cast<String, dynamic>());
    }
  }

  void _onError(Object err) {
    for (final c in _pending.values) {
      if (!c.isCompleted) c.completeError(err);
    }
    _pending.clear();
  }

  void _onDone() {
    _ch = null;
  }

  Future<Map<String, dynamic>> _send(Map<String, dynamic> packet) {
    final ch = _ch;
    if (ch == null) return Future.error(StateError('not connected'));
    // Find the id nested in the single top-level command.
    final cmd = packet.values.first as Map;
    final id = cmd['id'] as String?;
    ch.sink.add(jsonEncode(packet));
    if (id == null) return Future.value(<String, dynamic>{});
    final completer = Completer<Map<String, dynamic>>();
    _pending[id] = completer;
    return completer.future.timeout(const Duration(seconds: 15), onTimeout: () {
      _pending.remove(id);
      throw TimeoutException('Request $id timed out');
    });
  }

  static String _b64(String s) => base64.encode(utf8.encode(s));

  /// Login with login+password (basic scheme).
  Future<void> loginBasic(String login, String password) =>
      _login('basic', _b64('$login:$password'));

  /// Login with a previously issued session token.
  Future<void> loginToken(String token) => _login('token', token);

  /// Login with an external OIDC ID token (SSO).
  Future<void> loginOidc(String idToken) => _login('oidc', _b64(idToken));

  Future<void> _login(String scheme, String secret) async {
    final ctrl = await _send({
      'login': {'id': _nextId(), 'scheme': scheme, 'secret': secret},
    });
    final params = (ctrl['params'] as Map?)?.cast<String, dynamic>() ?? {};
    userId = params['user'] as String?;
    authToken = params['token'] as String?;
    tokenExpires = DateTime.tryParse(params['expires'] as String? ?? '');
  }

  /// Subscribe to a topic, optionally requesting description, subscriptions and recent data.
  Future<Map<String, dynamic>> subscribe(
    String topic, {
    bool wantDesc = true,
    bool wantSub = false,
    int? dataLimit,
  }) {
    final get = <String, dynamic>{};
    if (wantDesc) get['desc'] = <String, dynamic>{};
    if (wantSub) get['sub'] = <String, dynamic>{};
    if (dataLimit != null) get['data'] = {'limit': dataLimit};
    return _send({
      'sub': {
        'id': _nextId(),
        'topic': topic,
        if (get.isNotEmpty) 'get': get,
      },
    });
  }

  /// Fetch message history for a topic.
  Future<Map<String, dynamic>> getMessages(String topic, {int limit = 24, int? before}) {
    return _send({
      'get': {
        'id': _nextId(),
        'topic': topic,
        'what': 'data',
        'data': {'limit': limit, if (before != null) 'before': before},
      },
    });
  }

  /// Publish a message (text or Drafty content) to a topic.
  Future<Map<String, dynamic>> publish(
    String topic,
    dynamic content, {
    Map<String, dynamic>? head,
  }) {
    return _send({
      'pub': {
        'id': _nextId(),
        'topic': topic,
        'noecho': false,
        if (head != null) 'head': head,
        'content': content,
      },
    });
  }

  /// Send a soft notification: typing ('kp'), received ('recv') or read ('read').
  void note(String topic, String what, {int? seq}) {
    _send({
      'note': {'topic': topic, 'what': what, if (seq != null) 'seq': seq},
    });
  }

  void leave(String topic) {
    _send({
      'leave': {'id': _nextId(), 'topic': topic},
    });
  }

  /// Build an authorized download URL for a server file ref ("/v0/file/s/..").
  String fileUrl(String ref) {
    if (ref.startsWith('http')) return ref;
    final params = <String, String>{'apikey': apiKey};
    if (authToken != null) {
      params['auth'] = 'token';
      params['secret'] = authToken!;
    }
    final query = params.entries.map((e) => '${e.key}=${Uri.encodeComponent(e.value)}').join('&');
    return '$baseHttp$ref?$query';
  }

  void dispose() {
    _ch?.sink.close();
    _ch = null;
    _data.close();
    _meta.close();
    _pres.close();
    _info.close();
  }
}

class SunriseError implements Exception {
  final int code;
  final String message;
  SunriseError(this.code, this.message);
  @override
  String toString() => 'SunriseError($code): $message';
}
