import 'package:flutter/material.dart';

import '../sunrise/drafty.dart';
import '../sunrise/models.dart';
import '../sunrise/sunrise_client.dart';
import '../theme/glass_theme.dart';

/// Renders one message: text, image, video note, voice, file or call entry.
class MessageBubble extends StatelessWidget {
  const MessageBubble({super.key, required this.message, required this.isOwn, required this.client});

  final Message message;
  final bool isOwn;
  final SunriseClient client;

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

    final bubbleColor = isOwn ? const Color(0x338B5CF6) : Palette.glassFill;

    Widget body;
    switch (tp) {
      case 'image':
      case 'IM':
        final url = _src(data);
        body = url != null
            ? ClipRRect(
                borderRadius: BorderRadius.circular(12),
                child: Image.network(url, width: 240, fit: BoxFit.cover,
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
        body = Text(text, style: const TextStyle(color: Palette.textPrimary, fontSize: 14, height: 1.4));
    }

    return Align(
      alignment: isOwn ? Alignment.centerRight : Alignment.centerLeft,
      child: Container(
        constraints: const BoxConstraints(maxWidth: 320),
        margin: const EdgeInsets.symmetric(vertical: 3),
        padding: const EdgeInsets.fromLTRB(12, 10, 12, 6),
        decoration: BoxDecoration(
          color: bubbleColor,
          borderRadius: BorderRadius.circular(16),
          border: Border.all(color: Palette.glassBorder),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          mainAxisSize: MainAxisSize.min,
          children: [
            body,
            if (text.isNotEmpty && tp != 'text' && tp != 'call') ...[
              const SizedBox(height: 6),
              Text(text, style: const TextStyle(color: Palette.textPrimary, fontSize: 14)),
            ],
            const SizedBox(height: 2),
            Text(time, style: const TextStyle(color: Palette.textTertiary, fontSize: 11)),
          ],
        ),
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
