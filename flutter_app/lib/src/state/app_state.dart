import 'dart:async';

import 'package:flutter/foundation.dart';

import '../sunrise/drafty.dart';
import '../sunrise/models.dart';
import '../sunrise/sunrise_client.dart';
import 'call_controller.dart';
import 'livekit_room.dart';

enum Phase { loggedOut, connecting, ready }

/// Application state: owns the SunriseClient and exposes reactive UI state.
class AppState extends ChangeNotifier {
  AppState({SunriseClient? client}) : client = client ?? SunriseClient() {
    call = CallController(this.client);
    liveKit = LiveKitController(this.client);
    _wire();
  }

  final SunriseClient client;
  late final CallController call;
  late final LiveKitController liveKit;

  Phase phase = Phase.loggedOut;
  String? error;
  String displayName = 'You';

  final List<Contact> contacts = [];

  String? currentTopic;
  final List<Message> messages = [];
  bool peerTyping = false;
  Timer? _typingTimer;

  StreamSubscription? _dataSub, _metaSub, _presSub, _infoSub;

  void _wire() {
    _dataSub = client.onData.listen(_handleData);
    _metaSub = client.onMeta.listen(_handleMeta);
    _presSub = client.onPres.listen(_handlePres);
    _infoSub = client.onInfo.listen((info) {
      if (info['what'] == 'call') call.onSignal(info);
    });
  }

  // --- Auth --------------------------------------------------------------

  Future<void> loginBasic(String login, String password) async {
    await _withConnection(() async {
      await client.loginBasic(login, password);
      displayName = login;
    });
  }

  Future<void> loginOidc(String idToken) async {
    await _withConnection(() async {
      await client.loginOidc(idToken);
      displayName = client.userId ?? 'You';
    });
  }

  Future<void> _withConnection(Future<void> Function() body) async {
    phase = Phase.connecting;
    error = null;
    notifyListeners();
    try {
      await client.connect();
      await body();
      await _loadContacts();
      phase = Phase.ready;
    } catch (e) {
      error = e.toString();
      phase = Phase.loggedOut;
    }
    notifyListeners();
  }

  Future<void> _loadContacts() async {
    final meta = await client.subscribe('me', wantSub: true, wantDesc: true);
    // The subscribe response is a ctrl; the actual sub list arrives via {meta}.
    // Some servers also inline it; handle both.
    final subs = (meta['params'] as Map?)?['sub'];
    if (subs is List) _ingestSubs(subs);
  }

  // --- Conversations -----------------------------------------------------

  Future<void> openConversation(String topic) async {
    currentTopic = topic;
    messages.clear();
    peerTyping = false;
    notifyListeners();
    try {
      await client.subscribe(topic, dataLimit: 24);
      await client.getMessages(topic, limit: 24);
      client.note(topic, 'read');
    } catch (e) {
      error = e.toString();
      notifyListeners();
    }
  }

  /// Closes the open conversation (used by the phone-layout back button).
  void openConversationClear() {
    currentTopic = null;
    messages.clear();
    peerTyping = false;
    notifyListeners();
  }

  Future<void> sendText(String text) async {
    final topic = currentTopic;
    if (topic == null || text.trim().isEmpty) return;
    try {
      await client.publish(topic, text.trim());
    } catch (e) {
      error = e.toString();
      notifyListeners();
    }
  }

  void notifyTyping() {
    final topic = currentTopic;
    if (topic != null) client.note(topic, 'kp');
  }

  String _currentName() {
    final t = currentTopic;
    if (t == null) return 'Chat';
    return contacts.firstWhere((c) => c.topic == t, orElse: () => Contact(topic: t, name: t)).name;
  }

  // --- Calls -------------------------------------------------------------

  void startCall({required bool audioOnly}) {
    final t = currentTopic;
    if (t == null) return;
    call.startCall(t, _currentName(), audioOnly: audioOnly);
  }

  /// Start a group call via LiveKit (SFU). Surfaces an error if LiveKit isn't configured.
  Future<void> startGroupCall() async {
    final t = currentTopic;
    if (t == null) return;
    final joined = await liveKit.start(t, audioOnly: false);
    if (!joined) {
      error = 'Group calls require LiveKit, which is not configured on the server.';
      notifyListeners();
    }
  }

  // --- Media -------------------------------------------------------------

  Future<void> sendImage(List<int> bytes, String name, String mime, {int? width, int? height}) async {
    await _sendMedia(() async {
      final ref = await client.uploadFile(bytes, name, mime);
      return Drafty.image(ref: ref, mime: mime, width: width, height: height, name: name, size: bytes.length);
    });
  }

  Future<void> sendVideoNote(List<int> bytes, int durationMs, {String mime = 'video/mp4', int side = 240}) async {
    await _sendMedia(() async {
      final ref = await client.uploadFile(bytes, 'video-note.mp4', mime);
      return Drafty.videoNote(ref: ref, mime: mime, side: side, durationMs: durationMs, name: 'video-note.mp4', size: bytes.length);
    });
  }

  Future<void> sendVoice(List<int> bytes, int durationMs, {String mime = 'audio/m4a'}) async {
    await _sendMedia(() async {
      final ref = await client.uploadFile(bytes, 'voice-message.m4a', mime);
      return Drafty.audio(ref: ref, mime: mime, durationMs: durationMs, name: 'voice-message.m4a', size: bytes.length);
    });
  }

  Future<void> _sendMedia(Future<Map<String, dynamic>> Function() build) async {
    final topic = currentTopic;
    if (topic == null) return;
    try {
      final content = await build();
      await client.publish(topic, content);
    } catch (e) {
      error = e.toString();
      notifyListeners();
    }
  }

  // --- Server push handlers ---------------------------------------------

  void _handleData(Map<String, dynamic> data) {
    final msg = Message.fromData(data);

    // Incoming call detection (works regardless of the open conversation).
    final head = (data['head'] as Map?)?.cast<String, dynamic>();
    if (head?['webrtc'] == 'started' && msg.from != null && msg.from != client.userId) {
      final c = contacts.firstWhere((c) => c.topic == msg.topic, orElse: () => Contact(topic: msg.topic, name: msg.topic));
      call.handleIncoming(msg.topic, msg.seq, head?['aonly'] == true, c.name);
    }

    if (msg.topic == currentTopic) {
      final i = messages.indexWhere((m) => m.seq != 0 && m.seq == msg.seq);
      if (i >= 0) {
        messages[i] = msg;
      } else {
        messages.add(msg);
        messages.sort((a, b) => a.seq.compareTo(b.seq));
      }
      if (msg.from != null && msg.from != client.userId) {
        client.note(msg.topic, 'read', seq: msg.seq);
      }
      notifyListeners();
    }
    // Update the contact preview.
    final c = contacts.firstWhere((c) => c.topic == data['topic'], orElse: () => Contact(topic: '', name: ''));
    if (c.topic.isNotEmpty) {
      c.lastMessage = Drafty.preview(msg.content, head: msg.head);
      c.touched = msg.ts;
      _sortContacts();
      notifyListeners();
    }
  }

  void _handleMeta(Map<String, dynamic> meta) {
    final subs = meta['sub'];
    if (subs is List) {
      _ingestSubs(subs);
      notifyListeners();
    }
  }

  void _handlePres(Map<String, dynamic> pres) {
    final what = pres['what'] as String?;
    final src = pres['src'] as String?;
    if (what == 'kp' && src == currentTopic) {
      peerTyping = true;
      _typingTimer?.cancel();
      _typingTimer = Timer(const Duration(seconds: 3), () {
        peerTyping = false;
        notifyListeners();
      });
      notifyListeners();
    } else if ((what == 'on' || what == 'off') && src != null) {
      final c = contacts.firstWhere((c) => c.topic == src, orElse: () => Contact(topic: '', name: ''));
      if (c.topic.isNotEmpty) {
        c.online = what == 'on';
        notifyListeners();
      }
    }
  }

  void _ingestSubs(List subs) {
    for (final s in subs) {
      if (s is! Map) continue;
      final sub = Contact.fromSub(s.cast<String, dynamic>());
      if (sub.topic.isEmpty || sub.topic == 'me' || sub.topic == 'fnd') continue;
      final i = contacts.indexWhere((c) => c.topic == sub.topic);
      if (i >= 0) {
        contacts[i]
          ..name = sub.name
          ..unread = sub.unread
          ..online = sub.online
          ..touched = sub.touched ?? contacts[i].touched;
      } else {
        contacts.add(sub);
      }
    }
    _sortContacts();
  }

  void _sortContacts() {
    contacts.sort((a, b) => (b.touched ?? DateTime(0)).compareTo(a.touched ?? DateTime(0)));
  }

  void logout() {
    currentTopic = null;
    messages.clear();
    contacts.clear();
    phase = Phase.loggedOut;
    client.dispose();
    notifyListeners();
  }

  @override
  void dispose() {
    _dataSub?.cancel();
    _metaSub?.cancel();
    _presSub?.cancel();
    _infoSub?.cancel();
    _typingTimer?.cancel();
    call.dispose();
    liveKit.dispose();
    super.dispose();
  }
}
