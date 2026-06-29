import 'package:flutter/material.dart';

import '../state/app_state.dart';
import '../theme/glass_theme.dart';
import '../widgets/glass.dart';
import '../widgets/message_bubble.dart';

/// The message feed + composer for the selected conversation.
class ConversationScreen extends StatefulWidget {
  const ConversationScreen({super.key, required this.state, required this.title, this.onBack});
  final AppState state;
  final String title;
  final VoidCallback? onBack;

  @override
  State<ConversationScreen> createState() => _ConversationScreenState();
}

class _ConversationScreenState extends State<ConversationScreen> {
  final _input = TextEditingController();
  final _scroll = ScrollController();
  DateTime _lastTyping = DateTime.fromMillisecondsSinceEpoch(0);

  void _send() {
    final text = _input.text;
    if (text.trim().isEmpty) return;
    _input.clear();
    widget.state.sendText(text);
  }

  void _onChanged(String _) {
    final now = DateTime.now();
    if (now.difference(_lastTyping).inSeconds >= 3) {
      _lastTyping = now;
      widget.state.notifyTyping();
    }
  }

  @override
  void dispose() {
    _input.dispose();
    _scroll.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final s = widget.state;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (_scroll.hasClients) _scroll.jumpTo(_scroll.position.maxScrollExtent);
    });

    return Column(
      children: [
        // Header
        Padding(
          padding: const EdgeInsets.all(10),
          child: GlassPanel(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
            radius: 16,
            child: Row(
              children: [
                if (widget.onBack != null)
                  IconButton(
                      onPressed: widget.onBack,
                      icon: const Icon(Icons.arrow_back, color: Palette.textSecondary)),
                GlassAvatar(name: widget.title, size: 38),
                const SizedBox(width: 10),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(widget.title,
                          style: const TextStyle(
                              color: Palette.textPrimary, fontWeight: FontWeight.w600, fontSize: 15)),
                      Text(s.peerTyping ? 'typing…' : 'online',
                          style: const TextStyle(color: Palette.textTertiary, fontSize: 11)),
                    ],
                  ),
                ),
                IconButton(onPressed: () {}, icon: const Icon(Icons.call, color: Palette.textSecondary)),
                IconButton(onPressed: () {}, icon: const Icon(Icons.videocam, color: Palette.textSecondary)),
              ],
            ),
          ),
        ),
        // Messages
        Expanded(
          child: s.messages.isEmpty
              ? const Center(
                  child: Text('No messages yet. Say hello!',
                      style: TextStyle(color: Palette.textSecondary)))
              : ListView.builder(
                  controller: _scroll,
                  padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 8),
                  itemCount: s.messages.length,
                  itemBuilder: (ctx, i) {
                    final m = s.messages[i];
                    return MessageBubble(message: m, isOwn: m.from == s.client.userId, client: s.client);
                  },
                ),
        ),
        // Composer
        Padding(
          padding: const EdgeInsets.all(10),
          child: GlassPanel(
            padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 6),
            radius: 18,
            child: Row(
              children: [
                IconButton(onPressed: () {}, icon: const Icon(Icons.attach_file, color: Palette.textSecondary)),
                Expanded(
                  child: TextField(
                    controller: _input,
                    onChanged: _onChanged,
                    onSubmitted: (_) => _send(),
                    style: const TextStyle(color: Palette.textPrimary, fontSize: 14),
                    decoration: const InputDecoration(
                      hintText: 'Type a message…',
                      hintStyle: TextStyle(color: Palette.textTertiary),
                      border: InputBorder.none,
                    ),
                  ),
                ),
                IconButton(onPressed: () {}, icon: const Icon(Icons.mic, color: Palette.textSecondary)),
                IconButton(
                    onPressed: _send,
                    icon: const Icon(Icons.send_rounded, color: Palette.accent)),
              ],
            ),
          ),
        ),
      ],
    );
  }
}
