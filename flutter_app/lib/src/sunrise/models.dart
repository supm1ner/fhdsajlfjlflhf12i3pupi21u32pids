// Plain data models for the Sunrise messenger.

/// A single chat message.
class Message {
  final String topic;
  final String? from;
  final int seq;
  final DateTime ts;
  final Map<String, dynamic>? head;
  final dynamic content; // String or Drafty map {txt, fmt, ent}

  Message({
    required this.topic,
    required this.from,
    required this.seq,
    required this.ts,
    required this.head,
    required this.content,
  });

  factory Message.fromData(Map<String, dynamic> data) {
    return Message(
      topic: data['topic'] as String? ?? '',
      from: data['from'] as String?,
      seq: (data['seq'] as num?)?.toInt() ?? 0,
      ts: DateTime.tryParse(data['ts'] as String? ?? '')?.toLocal() ?? DateTime.now(),
      head: (data['head'] as Map?)?.cast<String, dynamic>(),
      content: data['content'],
    );
  }

  bool get isCall => head?['webrtc'] != null;
}

/// A conversation/contact derived from the `me` topic's subscriptions.
class Contact {
  final String topic;
  String name;
  String? photoRef;
  String lastMessage;
  int unread;
  bool online;
  DateTime? touched;

  Contact({
    required this.topic,
    required this.name,
    this.photoRef,
    this.lastMessage = '',
    this.unread = 0,
    this.online = false,
    this.touched,
  });

  factory Contact.fromSub(Map<String, dynamic> sub) {
    final pub = (sub['public'] as Map?)?.cast<String, dynamic>();
    final topic = sub['topic'] as String? ?? '';
    return Contact(
      topic: topic,
      name: (pub?['fn'] as String?)?.trim().isNotEmpty == true ? pub!['fn'] as String : topic,
      photoRef: _photoRef(pub),
      unread: (sub['unread'] as num?)?.toInt() ?? 0,
      online: sub['online'] as bool? ?? false,
      touched: DateTime.tryParse(sub['touched'] as String? ?? '')?.toLocal(),
    );
  }

  static String? _photoRef(Map<String, dynamic>? pub) {
    final photo = (pub?['photo'] as Map?)?.cast<String, dynamic>();
    return photo?['ref'] as String?;
  }
}
