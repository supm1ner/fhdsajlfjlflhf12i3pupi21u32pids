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
}
