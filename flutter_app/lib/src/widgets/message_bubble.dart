import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../sunrise/drafty.dart';
import '../sunrise/models.dart';
import '../sunrise/sunrise_client.dart';
import '../theme/glass_theme.dart';

/// Quick reactions offered in the picker (Telegram-style set), shared with the composer.
const List<String> kQuickReactions = ['👍', '❤️', '😂', '😮', '😢', '🙏', '🔥', '🎉'];

/// Renders one message: text, image, video note, voice, file or call entry.
class MessageBubble extends StatelessWidget {
  const MessageBubble({
    super.key,
    required this.message,
    required this.isOwn,
    required this.client,
    this.reactions = const {},
    this.myUserId,
    this.onReact,
    this.onLongPress,
    this.peerReadSeq = 0,
    this.highlight,
  });

  final Message message;
  final bool isOwn;
  final SunriseClient client;

  /// Aggregated reactions for this message: emoji -> set of reacting user ids.
  final Map<String, Set<String>> reactions;
  final String? myUserId;

  /// Toggle the current user's [emoji] reaction on this message.
  final void Function(String emoji)? onReact;

  /// Long-press handler (opens the reaction picker).
  final VoidCallback? onLongPress;

  /// Highest message seq the peer has read (drives delivery ticks on own messages).
  final int peerReadSeq;

  /// Active in-chat search term to highlight within the message text.
  final String? highlight;

  /// Renders [text] with the active search term highlighted, if any.
  Widget _highlighted(String text, TextStyle style) {
    final term = highlight;
    if (term == null || term.isEmpty) return Text(text, style: style);
    final lower = text.toLowerCase();
    final q = term.toLowerCase();
    if (!lower.contains(q)) return Text(text, style: style);
    final spans = <TextSpan>[];
    var pos = 0;
    while (true) {
      final idx = lower.indexOf(q, pos);
      if (idx < 0) {
        spans.add(TextSpan(text: text.substring(pos)));
        break;
      }
      if (idx > pos) spans.add(TextSpan(text: text.substring(pos, idx)));
      spans.add(TextSpan(
        text: text.substring(idx, idx + term.length),
        style: const TextStyle(backgroundColor: Color(0x8CFFD000)),
      ));
      pos = idx + term.length;
    }
    return Text.rich(TextSpan(style: style, children: spans));
  }

  String? _src(Map<String, dynamic>? data) {
    if (data == null) return null;
    final ref = data['ref'] as String?;
    if (ref != null) return client.fileUrl(ref);
    return null; // inline base64 omitted for brevity
  }

  @override
  Widget build(BuildContext context) {
    final tp = message.isCall ? 'call' : (Drafty.firstEntityType(message.content) ?? 'text');
    final data = Drafty.firstEntityData(message.content);
    final text = Drafty.plainText(message.content);
    final time = TimeOfDay.fromDateTime(message.ts).format(context);

    final bubbleColor = isOwn ? Palette.accent : Palette.surface;
    final textColor = isOwn ? Colors.white : Palette.textPrimary;
    final metaColor = isOwn ? Colors.white70 : Palette.textTertiary;

    Widget body;
    switch (tp) {
      case 'image':
      case 'IM':
        final url = _src(data);
        body = url != null
            ? ClipRRect(
                borderRadius: BorderRadius.circular(12),
                child: Image.network(url, width: 240, fit: BoxFit.cover,
                    loadingBuilder: (_, child, progress) =>
                        progress == null ? child : _placeholder('🖼'),
                    errorBuilder: (_, __, ___) => _placeholder('🖼')),
              )
            : _placeholder('🖼 Photo');
        break;
      case 'VD':
        final isNote = data?['width'] != null && data?['width'] == data?['height'];
        body = Container(
          width: isNote ? 200 : 240,
          height: isNote ? 200 : 150,
          decoration: BoxDecoration(
            color: Colors.black,
            shape: isNote ? BoxShape.circle : BoxShape.rectangle,
            borderRadius: isNote ? null : BorderRadius.circular(12),
          ),
          alignment: Alignment.center,
          child: const Icon(Icons.play_circle_fill, color: Colors.white70, size: 48),
        );
        break;
      case 'AU':
        body = Row(mainAxisSize: MainAxisSize.min, children: const [
          Icon(Icons.play_arrow_rounded, color: Palette.accent),
          SizedBox(width: 8),
          Text('Voice message', style: TextStyle(color: Palette.textPrimary)),
        ]);
        break;
      case 'EX':
        body = Row(mainAxisSize: MainAxisSize.min, children: [
          const Icon(Icons.attach_file, color: Palette.textSecondary),
          const SizedBox(width: 8),
          Flexible(child: Text(data?['name'] as String? ?? 'File',
              style: const TextStyle(color: Palette.textPrimary))),
        ]);
        break;
      case 'call':
        final st = message.head?['webrtc'] as String?;
        final missed = st == 'missed' || st == 'declined';
        body = Row(mainAxisSize: MainAxisSize.min, children: [
          Icon(Icons.call, size: 16, color: missed ? Palette.danger : Palette.textSecondary),
          const SizedBox(width: 8),
          Text(_callLabel(st), style: TextStyle(color: missed ? Palette.danger : Palette.textPrimary)),
        ]);
        break;
      default:
        body = _highlighted(text, TextStyle(color: textColor, fontSize: 14, height: 1.4));
    }

    // v0-style minimalist bubble: soft 18px corners with a small "tail" corner on the sender side.
    const r = Radius.circular(18);
    const tail = Radius.circular(5);
    final radius = BorderRadius.only(
      topLeft: r,
      topRight: r,
      bottomLeft: isOwn ? r : tail,
      bottomRight: isOwn ? tail : r,
    );
    return Align(
      alignment: isOwn ? Alignment.centerRight : Alignment.centerLeft,
      child: Column(
        crossAxisAlignment: isOwn ? CrossAxisAlignment.end : CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          GestureDetector(
            onLongPress: onLongPress == null
                ? null
                : () {
                    HapticFeedback.selectionClick();
                    onLongPress!();
                  },
            child: Container(
              constraints: BoxConstraints(
                  maxWidth: (MediaQuery.of(context).size.width * 0.72).clamp(220.0, 480.0)),
              margin: const EdgeInsets.symmetric(vertical: 2),
              padding: const EdgeInsets.fromLTRB(13, 9, 13, 7),
              decoration: BoxDecoration(
                color: bubbleColor,
                borderRadius: radius,
                border: isOwn ? null : Border.all(color: Palette.border),
              ),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                mainAxisSize: MainAxisSize.min,
                children: [
                  body,
                  if (text.isNotEmpty && tp != 'text' && tp != 'call') ...[
                    const SizedBox(height: 6),
                    _highlighted(text, TextStyle(color: textColor, fontSize: 14)),
                  ],
                  const SizedBox(height: 3),
                  Align(
                    alignment: Alignment.centerRight,
                    child: Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        Text(time, style: TextStyle(color: metaColor, fontSize: 10.5, letterSpacing: 0.2)),
                        if (isOwn && !message.isCall) ...[
                          const SizedBox(width: 3),
                          Icon(
                            (peerReadSeq > 0 && message.seq <= peerReadSeq) ? Icons.done_all : Icons.done,
                            size: 13,
                            color: (peerReadSeq > 0 && message.seq <= peerReadSeq) ? Colors.white : metaColor,
                          ),
                        ],
                      ],
                    ),
                  ),
                ],
              ),
            ),
          ),
          if (reactions.isNotEmpty) _reactionChips(),
        ],
      ),
    );
  }

  Widget _reactionChips() {
    final entries = reactions.entries.where((e) => e.value.isNotEmpty).toList()
      ..sort((a, b) => b.value.length.compareTo(a.value.length));
    return Padding(
      padding: const EdgeInsets.only(top: 3, bottom: 2),
      child: Wrap(
        spacing: 4,
        runSpacing: 4,
        children: entries.map((e) {
          final mine = myUserId != null && e.value.contains(myUserId);
          return GestureDetector(
            onTap: onReact == null ? null : () => onReact!(e.key),
            child: Container(
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
              decoration: BoxDecoration(
                color: mine ? Palette.accent : Palette.surface,
                borderRadius: BorderRadius.circular(12),
                border: Border.all(color: mine ? Palette.accent : Palette.border),
              ),
              child: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text(e.key, style: const TextStyle(fontSize: 13)),
                  const SizedBox(width: 4),
                  Text('${e.value.length}',
                      style: TextStyle(
                          fontSize: 12,
                          color: mine ? Colors.white : Palette.textSecondary,
                          fontWeight: FontWeight.w500)),
                ],
              ),
            ),
          );
        }).toList(),
      ),
    );
  }

  Widget _placeholder(String label) => Container(
        width: 240,
        height: 140,
        alignment: Alignment.center,
        decoration: BoxDecoration(color: Palette.glassFill, borderRadius: BorderRadius.circular(12)),
        child: Text(label, style: const TextStyle(fontSize: 28)),
      );

  String _callLabel(String? st) {
    switch (st) {
      case 'missed':
        return 'Missed call';
      case 'declined':
        return 'Call declined';
      case 'finished':
        return 'Call ended';
      case 'started':
        return 'Calling…';
      default:
        return 'Call';
    }
  }
}
