import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter/foundation.dart';
import 'package:path_provider/path_provider.dart';

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
  // Highest message seq the peer has read in the open conversation (0 = none yet).
  int peerReadSeq = 0;
  Timer? _typingTimer;

  StreamSubscription? _dataSub, _metaSub, _presSub, _infoSub;

  void _wire() {
    _dataSub = client.onData.listen(_handleData);
    _metaSub = client.onMeta.listen(_handleMeta);
    _presSub = client.onPres.listen(_handlePres);
    _infoSub = client.onInfo.listen((info) {
      final what = info['what'];
      if (what == 'call') {
        call.onSignal(info);
        return;
      }
      // The peer read our messages up to info.seq: drive the delivery ticks.
      if (what == 'read' && info['topic'] == currentTopic && info['from'] != client.userId) {
        final seq = (info['seq'] as num?)?.toInt() ?? 0;
        if (seq > peerReadSeq) {
          peerReadSeq = seq;
          notifyListeners();
        }
      }
    });
  }

  // --- Auth --------------------------------------------------------------

  Future<void> loginBasic(String login, String password) async {
    await _withConnection(() async {
      await client.loginBasic(login, password);
      displayName = login;
    });
  }

  Future<void> register(String login, String password) async {
    await _withConnection(() async {
      await client.register(login, password);
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
      _rewire();
      await body();
      await _loadContacts();
      await _restoreAndSubscribe();
      phase = Phase.ready;
    } catch (e) {
      error = e.toString();
      phase = Phase.loggedOut;
    }
    notifyListeners();
  }

  void _rewire() {
    _dataSub?.cancel();
    _metaSub?.cancel();
    _presSub?.cancel();
    _infoSub?.cancel();
    _wire();
  }

  Future<void> _loadContacts() async {
    final meta = await client.subscribe('me', wantSub: true, wantDesc: true);
    // The subscribe response is a ctrl; the actual sub list arrives via {meta}.
    // Some servers also inline it; handle both.
    final subs = (meta['params'] as Map?)?['sub'];
    if (subs is List) _ingestSubs(subs);
  }

  // --- Search -------------------------------------------------------------

  List<Map<String, dynamic>> searchResults = [];
  bool searchBusy = false;

  Future<void> searchUsers(String query) async {
    searchBusy = true;
    notifyListeners();
    try {
      searchResults = await client.searchUsers(query);
    } catch (e) {
      error = e.toString();
      searchResults = [];
    }
    searchBusy = false;
    notifyListeners();
  }

  void clearSearch() {
    searchResults = [];
    notifyListeners();
  }

  /// Open a conversation with a user found via search.
  Future<void> openConversationWith(String userTopic, {String? name}) async {
    currentTopic = userTopic;
    messages.clear();
    peerTyping = false;
    peerReadSeq = 0;

    // Add to contacts if not already there.
    if (!contacts.any((c) => c.topic == userTopic)) {
      contacts.add(Contact(
        topic: userTopic,
        name: name ?? userTopic,
        touched: DateTime.now(),
      ));
      _sortContacts();
      _persistContacts();
    }

    notifyListeners();
    try {
      await client.subscribe(userTopic, dataLimit: 24);
      await client.getMessages(userTopic, limit: 24);
      client.note(userTopic, 'read');
    } catch (e) {
      error = e.toString();
      notifyListeners();
    }
  }

  // --- Conversations -----------------------------------------------------

  Future<void> openConversation(String topic) async {
    currentTopic = topic;
    messages.clear();
    peerTyping = false;
    peerReadSeq = 0;
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
    peerReadSeq = 0;
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

  /// Send [text], converting recorded "@Name" tokens into Drafty mentions.
  Future<void> sendMentionText(String text, List<Map<String, String>> mentions) async {
    final topic = currentTopic;
    final trimmed = text.trim();
    if (topic == null || trimmed.isEmpty) return;
    final content = Drafty.withMentions(trimmed, mentions) ?? trimmed;
    try {
      await client.publish(topic, content);
    } catch (e) {
      error = e.toString();
      notifyListeners();
    }
  }

  // --- Reactions ---------------------------------------------------------

  /// A reaction message carries its target message's seq in head.react_to.
  bool _isReaction(Message m) => m.head?['react_to'] != null;

  /// Messages shown in the feed: everything except reaction metadata.
  List<Message> get visibleMessages => messages.where((m) => !_isReaction(m)).toList();

  /// Aggregated reactions for a target message: emoji -> set of reacting user ids.
  /// Add/remove operations are applied in seq order (messages are kept sorted).
  Map<String, Set<String>> reactionsFor(int seq) {
    final result = <String, Set<String>>{};
    for (final m in messages) {
      final head = m.head;
      if (head == null) continue;
      final target = head['react_to'];
      final emoji = head['react'] as String?;
      if (target == null || emoji == null) continue;
      final t = target is num ? target.toInt() : int.tryParse('$target');
      if (t != seq) continue;
      final users = result.putIfAbsent(emoji, () => <String>{});
      if (head['react_op'] == 'remove') {
        users.remove(m.from ?? '');
      } else {
        users.add(m.from ?? '');
      }
    }
    result.removeWhere((_, users) => users.isEmpty);
    return result;
  }

  /// Toggle the current user's [emoji] reaction on message [seq].
  Future<void> toggleReaction(int seq, String emoji) async {
    final topic = currentTopic;
    if (topic == null) return;
    final mine = (reactionsFor(seq)[emoji] ?? const <String>{}).contains(client.userId);
    try {
      await client.publish(topic, emoji, head: {
        'react_to': '$seq',
        'react': emoji,
        'react_op': mine ? 'remove' : 'add',
      });
    } catch (e) {
      error = e.toString();
      notifyListeners();
    }
  }

  String _currentName() {
    final t = currentTopic;
    if (t == null) return 'Chat';
    return contacts.firstWhere((c) => c.topic == t, orElse: () => Contact(topic: t, name: t)).name;
  }

  /// Whether the peer of the open conversation is currently online.
  bool get peerOnline {
    final t = currentTopic;
    if (t == null) return false;
    for (final c in contacts) {
      if (c.topic == t) return c.online;
    }
    return false;
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
    // Reactions are metadata: don't surface them in the contact preview.
    if (_isReaction(msg)) return;
    // Update the contact preview — create if missing.
    var idx = contacts.indexWhere((c) => c.topic == data['topic']);
    if (idx < 0) {
      contacts.add(Contact(topic: msg.topic, name: msg.topic));
      idx = contacts.length - 1;
    }
    final c = contacts[idx];
    c.lastMessage = Drafty.preview(msg.content, head: msg.head);
    c.touched = msg.ts;
    _sortContacts();
    _persistContacts();
    notifyListeners();
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
    _persistContacts();
  }

  void _sortContacts() {
    contacts.sort((a, b) => (b.touched ?? DateTime(0)).compareTo(a.touched ?? DateTime(0)));
  }

  // --- Local persistence ---------------------------------------------------

  Future<File> get _contactsFile async {
    final dir = await getApplicationDocumentsDirectory();
    return File('${dir.path}/contacts.json');
  }

  Future<void> _persistContacts() async {
    try {
      final data = contacts.map((c) => {
        'topic': c.topic,
        'name': c.name,
        'photoRef': c.photoRef,
        'lastMessage': c.lastMessage,
        'unread': c.unread,
        'touched': c.touched?.toIso8601String(),
      }).toList();
      final file = await _contactsFile;
      await file.writeAsString(jsonEncode(data));
    } catch (_) {}
  }

  Future<void> _restoreContacts() async {
    try {
      final file = await _contactsFile;
      if (!await file.exists()) return;
      final data = jsonDecode(await file.readAsString()) as List;
      for (final m in data) {
        if (m is! Map) continue;
        final topic = m['topic'] as String? ?? '';
        if (topic.isEmpty) continue;
        final i = contacts.indexWhere((c) => c.topic == topic);
        if (i < 0) {
          contacts.add(Contact(
            topic: topic,
            name: m['name'] as String? ?? topic,
            photoRef: m['photoRef'] as String?,
            lastMessage: m['lastMessage'] as String? ?? '',
            touched: DateTime.tryParse(m['touched'] as String? ?? ''),
          ));
        }
      }
      _sortContacts();
    } catch (_) {}
  }

  /// After login, load persisted contacts and subscribe to each p2p topic
  /// so messages are delivered in real time.
  Future<void> _restoreAndSubscribe() async {
    await _restoreContacts();
    notifyListeners();
    for (final c in List.of(contacts)) {
      try {
        await client.subscribe(c.topic, dataLimit: 24);
        await client.getMessages(c.topic, limit: 24);
        client.note(c.topic, 'read');
      } catch (_) {}
    }
    notifyListeners();
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
