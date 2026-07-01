import 'dart:async';
import 'dart:convert';
import 'dart:io';

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
  }) {
    _initStreams();
  }

  void _initStreams() {
    _data = StreamController<Map<String, dynamic>>.broadcast();
    _meta = StreamController<Map<String, dynamic>>.broadcast();
    _pres = StreamController<Map<String, dynamic>>.broadcast();
    _info = StreamController<Map<String, dynamic>>.broadcast();
  }

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

  late StreamController<Map<String, dynamic>> _data;
  late StreamController<Map<String, dynamic>> _meta;
  late StreamController<Map<String, dynamic>> _pres;
  late StreamController<Map<String, dynamic>> _info;

  Stream<Map<String, dynamic>> get onData => _data.stream;
  Stream<Map<String, dynamic>> get onMeta => _meta.stream;
  Stream<Map<String, dynamic>> get onPres => _pres.stream;
  Stream<Map<String, dynamic>> get onInfo => _info.stream;

  bool _closed = false;
  bool get isConnected => _ch != null && !_closed;

  String get _scheme => secure ? 'wss' : 'ws';
  String get baseHttp => '${secure ? 'https' : 'http'}://$host';
  String get wsUrl => '$_scheme://$host/v0/channels?apikey=$apiKey';

  String _nextId() => (++_id).toString();

  /// Opens the WebSocket and performs the {hi} handshake.
  Future<void> connect() async {
    if (_ch != null) return;
    _closed = false;
    _fndSubscribed = false;
    _initStreams();
    final ch = WebSocketChannel.connect(Uri.parse(wsUrl));
    _ch = ch;
    ch.stream.listen(_onMessage, onError: _onError, onDone: _onDone, cancelOnError: false);
    await ch.ready;
    await _send({
      'hi': {'id': _nextId(), 'ver': '0.22', 'ua': appName},
    });
  }

  void _onMessage(dynamic raw) {
    if (_closed) return;
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
    if (_closed) return;
    _closed = true;
    for (final c in _pending.values) {
      if (!c.isCompleted) c.completeError(StateError('connection closed'));
    }
    _pending.clear();
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

  /// Register a new account (basic auth: login + password).
  Future<void> register(String login, String password) async {
    final ctrl = await _send({
      'acc': {
        'id': _nextId(),
        'user': 'new',
        'scheme': 'basic',
        'secret': _b64('$login:$password'),
        'login': true,
        'desc': {'public': {'fn': login}},
      },
    });
    final params = (ctrl['params'] as Map?)?.cast<String, dynamic>() ?? {};
    userId = params['user'] as String?;
    authToken = params['token'] as String?;
    tokenExpires = DateTime.tryParse(params['expires'] as String? ?? '');
  }

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

  /// Send a WebRTC call signaling event over a topic.
  /// [event] is one of: ringing, accept, offer, answer, ice-candidate, hang-up.
  void videoCall(String topic, String event, int seq, [dynamic payload]) {
    _send({
      'note': {
        'topic': topic,
        'what': 'call',
        'seq': seq,
        'event': event,
        if (payload != null) 'payload': payload,
      },
    });
  }

  /// Request a LiveKit access token for [room]. Returns { url, token, room, identity }.
  /// Throws [LiveKitNotConfigured] when the server has no LiveKit configured (HTTP 501).
  Future<Map<String, dynamic>> fetchLiveKitToken(String room) async {
    final params = <String, String>{'room': room, 'apikey': apiKey};
    if (authToken != null) {
      params['auth'] = 'token';
      params['secret'] = authToken!;
    }
    final query = params.entries.map((e) => '${e.key}=${Uri.encodeComponent(e.value)}').join('&');
    final uri = Uri.parse('$baseHttp/v0/livekit/token?$query');
    final http = HttpClient();
    try {
      final resp = await (await http.getUrl(uri)).close();
      final text = await resp.transform(utf8.decoder).join();
      if (resp.statusCode == 501) throw LiveKitNotConfigured();
      if (resp.statusCode < 200 || resp.statusCode >= 300) {
        throw Exception('LiveKit token request failed: ${resp.statusCode} $text');
      }
      return jsonDecode(text) as Map<String, dynamic>;
    } finally {
      http.close();
    }
  }

  /// Upload a file to the server; returns the file ref ("/v0/file/s/..").
  Future<String> uploadFile(List<int> bytes, String filename, String mimeType) async {
    final uri = Uri.parse('$baseHttp/v0/file/u');
    final boundary = '----sunrise${DateTime.now().microsecondsSinceEpoch}';
    final client = HttpClient();
    try {
      final req = await client.postUrl(uri);
      req.headers.set(HttpHeaders.contentTypeHeader, 'multipart/form-data; boundary=$boundary');
      req.headers.set('X-Sunrise-APIKey', apiKey);
      if (authToken != null) {
        req.headers.set(HttpHeaders.authorizationHeader, 'Token $authToken');
      }
      final head = utf8.encode(
        '--$boundary\r\n'
        'Content-Disposition: form-data; name="file"; filename="$filename"\r\n'
        'Content-Type: $mimeType\r\n\r\n',
      );
      final tail = utf8.encode('\r\n--$boundary--\r\n');
      req.add(head);
      req.add(bytes);
      req.add(tail);
      final resp = await req.close();
      final text = await resp.transform(utf8.decoder).join();
      if (resp.statusCode < 200 || resp.statusCode >= 300) {
        throw Exception('Upload failed: ${resp.statusCode} $text');
      }
      final json = jsonDecode(text) as Map<String, dynamic>;
      final params = ((json['ctrl'] as Map?)?['params'] as Map?)?.cast<String, dynamic>();
      final url = params?['url'] as String?;
      if (url == null) throw Exception('Upload response missing url');
      return url;
    } finally {
      client.close();
    }
  }

  void leave(String topic) {
    _send({
      'leave': {'id': _nextId(), 'topic': topic},
    });
  }

  // --- User search (fnd topic) -----------------------------------------------

  bool _fndSubscribed = false;

  /// Search users by query string (e.g. "alice", "email:alice@example.com").
  /// Returns a list of maps: {topic, name, online}.
  Future<List<Map<String, dynamic>>> searchUsers(String query) async {
    if (query.trim().isEmpty) return [];

    final ch = _ch;
    if (ch == null) return [];

    // Subscribe to fnd topic once.
    if (!_fndSubscribed) {
      await _send({
        'sub': {
          'id': _nextId(),
          'topic': 'fnd',
        },
      });
      _fndSubscribed = true;
    }

    // Format query: search with basic: prefix since tags are stored as "basic:<username>".
    final q = query.trim();
    final fq = '$q,basic:$q';

    // Set the search query via desc.public.
    await _send({
      'set': {
        'id': _nextId(),
        'topic': 'fnd',
        'desc': {'public': fq},
      },
    });

    // Listen for meta response BEFORE sending get.
    final metaCompleter = Completer<Map<String, dynamic>>();
    late final StreamSubscription metaSub;
    metaSub = _meta.stream.listen((meta) {
      if (!metaCompleter.isCompleted) {
        metaCompleter.complete(meta);
      }
    });

    try {
      // Send get (ctrl is just an ack, we care about the meta).
      final getId = _nextId();
      ch.sink.add(jsonEncode({
        'get': {
          'id': getId,
          'topic': 'fnd',
          'what': 'sub',
        },
      }));

      // Wait for meta response with results.
      final meta = await metaCompleter.future.timeout(
          const Duration(seconds: 10), onTimeout: () => <String, dynamic>{});

      final results = <Map<String, dynamic>>[];
      final subs = (meta['sub'] as List?) ?? [];
      for (final s in subs) {
        if (s is! Map) continue;
        final m = s.cast<String, dynamic>();
        final topic = m['user'] as String? ?? m['topic'] as String? ?? '';
        if (topic.isEmpty) continue;
        final pub = m['public'];
        final name = (pub is Map) ? (pub['fn'] as String? ?? topic) : topic;
        results.add({'topic': topic, 'name': name, 'online': m['online'] == true});
      }
      return results;
    } finally {
      metaSub.cancel();
    }
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
    _closed = true;
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

/// Thrown when the backend has no LiveKit configured (clients fall back to 1:1/mesh).
class LiveKitNotConfigured implements Exception {
  @override
  String toString() => 'LiveKit is not configured on the server';
}
