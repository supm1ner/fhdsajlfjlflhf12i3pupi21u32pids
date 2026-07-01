// Minimal Drafty helpers: extract plain text and a short preview from message content.
// Content is either a plain String or a Drafty map {txt, fmt, ent}.

class Drafty {
  /// Returns the plain text of a Drafty/string message.
  static String plainText(dynamic content) {
    if (content == null) return '';
    if (content is String) return content;
    if (content is Map) return (content['txt'] as String?)?.trim() ?? '';
    return '';
  }

  /// Returns the first media entity type in a Drafty message, or null.
  static String? firstEntityType(dynamic content) {
    if (content is! Map) return null;
    final ent = content['ent'];
    if (ent is List && ent.isNotEmpty) {
      final first = ent.first;
      if (first is Map) return first['tp'] as String?;
    }
    return null;
  }

  /// Returns the data map of the first entity, or null.
  static Map<String, dynamic>? firstEntityData(dynamic content) {
    if (content is! Map) return null;
    final ent = content['ent'];
    if (ent is List && ent.isNotEmpty && ent.first is Map) {
      return (ent.first['data'] as Map?)?.cast<String, dynamic>();
    }
    return null;
  }

  /// A short, human-readable preview for contact lists.
  static String preview(dynamic content, {Map<String, dynamic>? head}) {
    if (head?['webrtc'] != null) return '📞 Call';
    final tp = firstEntityType(content);
    switch (tp) {
      case 'IM':
        return '🖼 Photo';
      case 'VD':
        final d = firstEntityData(content);
        return (d?['width'] != null && d?['width'] == d?['height']) ? '⭕ Video note' : '🎬 Video';
      case 'AU':
        return '🎤 Voice message';
      case 'EX':
        final d = firstEntityData(content);
        return '📎 ${d?['name'] ?? 'File'}';
      case 'VC':
        return '📞 Call';
    }
    final txt = plainText(content);
    return txt.isEmpty ? '' : txt;
  }

  // --- Builders (match the JS SDK Drafty format) -------------------------

  static Map<String, dynamic> _single(String tp, Map<String, dynamic> data) => {
        'txt': ' ',
        'fmt': [
          {'at': 0, 'len': 1, 'key': 0}
        ],
        'ent': [
          {'tp': tp, 'data': data}
        ],
      };

  /// Content for a video-call invite (VC entity).
  static Map<String, dynamic> videoCall(bool audioOnly) => _single('VC', {'aonly': audioOnly});

  static Map<String, dynamic> image({
    required String ref,
    required String mime,
    int? width,
    int? height,
    String? name,
    int? size,
  }) =>
      _single('IM', {'mime': mime, 'ref': ref, 'width': width, 'height': height, 'name': name, 'size': size});

  /// A square video (rendered as a round "video note").
  static Map<String, dynamic> videoNote({
    required String ref,
    required String mime,
    int side = 240,
    int? durationMs,
    String? name,
    int? size,
  }) =>
      _single('VD', {
        'mime': mime,
        'ref': ref,
        'width': side,
        'height': side,
        'duration': durationMs ?? 0,
        'name': name,
        'size': size,
      });

  /// Builds a Drafty document from [text], turning each "@Name" token that matches
  /// one of [mentions] ({'name','uid'}) into an MN (mention) entity. Returns null when
  /// no mention token is present, so the caller can send plain text instead.
  static Map<String, dynamic>? withMentions(String text, List<Map<String, String>> mentions) {
    if (mentions.isEmpty) return null;
    // Longest names first so "@Anna" wins over "@Ann".
    final toks = [...mentions]..sort((a, b) => (b['name'] ?? '').length.compareTo((a['name'] ?? '').length));
    final boundary = RegExp(r'[\s.,!?;:)]');
    final fmt = <Map<String, dynamic>>[];
    final ent = <Map<String, dynamic>>[];
    var i = 0;
    while (i < text.length) {
      Map<String, String>? matched;
      for (final m in toks) {
        final tok = '@${m['name']}';
        if (text.startsWith(tok, i)) {
          final nextIdx = i + tok.length;
          final nextCh = nextIdx < text.length ? text[nextIdx] : null;
          if (nextCh == null || boundary.hasMatch(nextCh)) {
            matched = m;
            break;
          }
        }
      }
      if (matched != null) {
        final tok = '@${matched['name']}';
        fmt.add({'at': i, 'len': tok.length, 'key': ent.length});
        ent.add({'tp': 'MN', 'data': {'val': matched['uid']}});
        i += tok.length;
      } else {
        i++;
      }
    }
    if (ent.isEmpty) return null;
    return {'txt': text, 'fmt': fmt, 'ent': ent};
  }

  static Map<String, dynamic> audio({
    required String ref,
    required String mime,
    int? durationMs,
    String? name,
    int? size,
  }) =>
      _single('AU', {'mime': mime, 'ref': ref, 'duration': durationMs ?? 0, 'name': name, 'size': size});
}
