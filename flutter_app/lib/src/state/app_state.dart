import 'dart:async';

import 'package:flutter/foundation.dart';

import '../sunrise/drafty.dart';
import '../sunrise/models.dart';
import '../sunrise/sunrise_client.dart';

enum Phase { loggedOut, connecting, ready }

/// Application state: owns the SunriseClient and exposes reactive UI state.
class AppState extends ChangeNotifier {
  AppState({SunriseClient? client}) : client = client ?? SunriseClient() {
    _wire();
  }

  final SunriseClient client;

  Phase phase = Phase.loggedOut;
  String? error;
  String displayName = 'You';

  final List<Contact> contacts = [];

  String? currentTopic;
  final List<Message> messages = [];
  bool peerTyping = false;
  Timer? _typingTimer;

  StreamSubscription? _dataSub, _metaSub, _presSub;

  void _wire() {
    _dataSub = client.onData.listen(_handleData);
    _metaSub = client.onMeta.listen(_handleMeta);
    _presSub = client.onPres.listen(_handlePres);
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

  // --- Server push handlers ---------------------------------------------

  void _handleData(Map<String, dynamic> data) {
    final msg = Message.fromData(data);
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
    _typingTimer?.cancel();
    super.dispose();
  }
}
