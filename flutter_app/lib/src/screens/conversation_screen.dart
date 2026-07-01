import 'dart:async';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:image_picker/image_picker.dart';
import 'package:path_provider/path_provider.dart';
import 'package:record/record.dart';

import '../state/app_state.dart';
import '../theme/glass_theme.dart';
import '../widgets/glass.dart';
import '../widgets/message_bubble.dart';
import 'video_note_recorder.dart';

/// Emoji offered in the composer picker.
const List<String> kComposerEmoji = [
  '😀', '😁', '😂', '🤣', '😊', '😍', '😘', '😉', '😎', '🥰', '😇', '🙂', '🙃', '😌', '😔', '😒',
  '😢', '😭', '😤', '😠', '😡', '🥺', '😳', '😱', '🤔', '🤨', '😐', '😴', '🤗', '🤭', '🥳', '😜',
  '👍', '👎', '👌', '✌️', '🤞', '🤟', '👏', '🙌', '🙏', '💪', '👋', '🤝', '👀', '🫶', '🤦', '🤷',
  '❤️', '🧡', '💛', '💚', '💙', '💜', '🖤', '🤍', '💔', '💕', '💖', '💯', '🔥', '✨', '🎉', '🎊',
  '🎁', '🏆', '⭐', '🌟', '💡', '✅', '❌', '⚡', '🚀', '🎯', '🌸', '🌹', '🌈', '☀️', '🌙', '🍀',
  '🐶', '🐱', '🦊', '🐻', '🐼', '🦄', '🍕', '🍔', '🍟', '🍣', '🎂', '☕', '🍺', '🥂', '🍎', '🍓',
];

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
  final _picker = ImagePicker();
  final _recorder = AudioRecorder();
  DateTime _lastTyping = DateTime.fromMillisecondsSinceEpoch(0);
  bool _recordingVoice = false;
  final _voiceWatch = Stopwatch();
  Timer? _voiceTicker;
  int _lastMsgCount = 0;
  bool _didInitialScroll = false;

  void _send() {
    final text = _input.text;
    if (text.trim().isEmpty) return;
    _input.clear();
    widget.state.sendText(text);
  }

  // Insert [emoji] at the current caret position in the composer.
  void _insertEmoji(String emoji) {
    final sel = _input.selection;
    final text = _input.text;
    final start = sel.start >= 0 ? sel.start : text.length;
    final end = sel.end >= 0 ? sel.end : text.length;
    final next = text.replaceRange(start, end, emoji);
    _input.value = TextEditingValue(
      text: next,
      selection: TextSelection.collapsed(offset: start + emoji.length),
    );
  }

  Future<void> _showEmojiPicker() async {
    await showModalBottomSheet<void>(
      context: context,
      backgroundColor: Palette.bg,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
      ),
      builder: (ctx) => SafeArea(
        child: SizedBox(
          height: 280,
          child: GridView.count(
            crossAxisCount: 8,
            padding: const EdgeInsets.all(12),
            children: kComposerEmoji
                .map((emoji) => InkWell(
                      borderRadius: BorderRadius.circular(10),
                      onTap: () => _insertEmoji(emoji),
                      child: Center(child: Text(emoji, style: const TextStyle(fontSize: 26))),
                    ))
                .toList(),
          ),
        ),
      ),
    );
  }

  Future<void> _showReactionPicker(AppState s, int seq) async {
    await showModalBottomSheet<void>(
      context: context,
      backgroundColor: Palette.bg,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
      ),
      builder: (ctx) => SafeArea(
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 20),
          child: Wrap(
            alignment: WrapAlignment.center,
            spacing: 8,
            runSpacing: 8,
            children: kQuickReactions.map((emoji) {
              return InkWell(
                borderRadius: BorderRadius.circular(12),
                onTap: () {
                  Navigator.of(ctx).pop();
                  s.toggleReaction(seq, emoji);
                },
                child: Padding(
                  padding: const EdgeInsets.all(8),
                  child: Text(emoji, style: const TextStyle(fontSize: 30)),
                ),
              );
            }).toList(),
          ),
        ),
      ),
    );
  }

  void _onChanged(String _) {
    final now = DateTime.now();
    if (now.difference(_lastTyping).inSeconds >= 3) {
      _lastTyping = now;
      widget.state.notifyTyping();
    }
  }

  Future<void> _attachImage() async {
    final file = await _picker.pickImage(source: ImageSource.gallery, imageQuality: 85);
    if (file == null) return;
    final bytes = await file.readAsBytes();
    await widget.state.sendImage(bytes, file.name, 'image/jpeg');
  }

  Future<void> _recordVideoNote() async {
    final result = await Navigator.of(context).push<VideoNoteResult>(
      MaterialPageRoute(builder: (_) => const VideoNoteRecorderScreen(), fullscreenDialog: true),
    );
    if (result != null) {
      await widget.state.sendVideoNote(result.bytes, result.durationMs);
    }
  }

  Future<void> _toggleVoice() async {
    if (_recordingVoice) {
      final path = await _recorder.stop();
      _voiceWatch.stop();
      _voiceTicker?.cancel();
      setState(() => _recordingVoice = false);
      if (path != null) {
        final bytes = await File(path).readAsBytes();
        await widget.state.sendVoice(bytes, _voiceWatch.elapsedMilliseconds);
      }
      return;
    }
    if (!await _recorder.hasPermission()) return;
    final dir = await getTemporaryDirectory();
    final path = '${dir.path}/voice_${DateTime.now().millisecondsSinceEpoch}.m4a';
    await _recorder.start(const RecordConfig(), path: path);
    _voiceWatch
      ..reset()
      ..start();
    // Refresh the elapsed-time label roughly twice a second while recording.
    _voiceTicker?.cancel();
    _voiceTicker = Timer.periodic(const Duration(milliseconds: 500), (_) {
      if (mounted && _recordingVoice) setState(() {});
    });
    setState(() => _recordingVoice = true);
  }

  static String _fmtElapsed(Duration d) {
    final m = d.inMinutes.remainder(60).toString().padLeft(1, '0');
    final s = d.inSeconds.remainder(60).toString().padLeft(2, '0');
    return '$m:$s';
  }

  @override
  void dispose() {
    _input.dispose();
    _scroll.dispose();
    _recorder.dispose();
    _voiceTicker?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final s = widget.state;
    final msgs = s.visibleMessages;
    // Auto-scroll to the newest message only when new messages arrived AND the user
    // is already near the bottom — never yank the view while they're reading history.
    final grew = msgs.length > _lastMsgCount;
    // A shrinking list means the conversation was switched/cleared: re-anchor to bottom.
    if (msgs.length < _lastMsgCount) _didInitialScroll = false;
    _lastMsgCount = msgs.length;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!_scroll.hasClients) return;
      final pos = _scroll.position;
      if (!_didInitialScroll && msgs.isNotEmpty) {
        _scroll.jumpTo(pos.maxScrollExtent);
        _didInitialScroll = true;
        return;
      }
      final nearBottom = pos.maxScrollExtent - pos.pixels < 240;
      if (grew && nearBottom) _scroll.jumpTo(pos.maxScrollExtent);
    });

    return Column(
      children: [
        _header(s),
        Expanded(
          child: msgs.isEmpty
              ? const Center(child: Text('No messages yet. Say hello!', style: TextStyle(color: Palette.textSecondary)))
              : Builder(builder: (ctx) {
                  return ListView.builder(
                    controller: _scroll,
                    padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 8),
                    itemCount: msgs.length,
                    itemBuilder: (ctx, i) {
                      final m = msgs[i];
                      return MessageBubble(
                        message: m,
                        isOwn: m.from == s.client.userId,
                        client: s.client,
                        reactions: s.reactionsFor(m.seq),
                        myUserId: s.client.userId,
                        onReact: (emoji) => s.toggleReaction(m.seq, emoji),
                        onLongPress: () => _showReactionPicker(s, m.seq),
                      );
                    },
                  );
                }),
        ),
        _composer(),
      ],
    );
  }

  Widget _header(AppState s) => Padding(
        padding: const EdgeInsets.all(10),
        child: GlassPanel(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
          radius: 16,
          child: Row(
            children: [
              if (widget.onBack != null)
                IconButton(onPressed: widget.onBack, icon: const Icon(Icons.arrow_back, color: Palette.textSecondary)),
              GlassAvatar(name: widget.title, size: 38),
              const SizedBox(width: 10),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(widget.title,
                        style: const TextStyle(color: Palette.textPrimary, fontWeight: FontWeight.w600, fontSize: 15)),
                    Text(s.peerTyping ? 'typing…' : (s.peerOnline ? 'online' : 'offline'),
                        style: TextStyle(
                            color: s.peerOnline ? Palette.accent : Palette.textTertiary, fontSize: 11)),
                  ],
                ),
              ),
              IconButton(
                  onPressed: () => s.startCall(audioOnly: true),
                  icon: const Icon(Icons.call, color: Palette.textSecondary)),
              IconButton(
                  onPressed: () => s.startCall(audioOnly: false),
                  icon: const Icon(Icons.videocam, color: Palette.textSecondary)),
              IconButton(
                  onPressed: s.startGroupCall,
                  icon: const Icon(Icons.groups, color: Palette.textSecondary)),
            ],
          ),
        ),
      );

  Widget _composer() => Padding(
        padding: const EdgeInsets.all(10),
        child: GlassPanel(
          padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 4),
          radius: 18,
          child: _recordingVoice
              ? Row(children: [
                  const SizedBox(width: 8),
                  const Icon(Icons.fiber_manual_record, color: Palette.danger, size: 14),
                  const SizedBox(width: 8),
                  Expanded(
                      child: Text('Recording  ${_fmtElapsed(_voiceWatch.elapsed)}',
                          style: const TextStyle(color: Palette.textSecondary))),
                  IconButton(onPressed: _toggleVoice, icon: const Icon(Icons.send_rounded, color: Palette.accent)),
                ])
              : Row(
                  children: [
                    IconButton(onPressed: _attachImage, icon: const Icon(Icons.image_outlined, color: Palette.textSecondary)),
                    IconButton(onPressed: _showEmojiPicker, icon: const Icon(Icons.emoji_emotions_outlined, color: Palette.textSecondary)),
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
                    IconButton(onPressed: _toggleVoice, icon: const Icon(Icons.mic_none, color: Palette.textSecondary)),
                    IconButton(onPressed: _recordVideoNote, icon: const Icon(Icons.circle_outlined, color: Palette.textSecondary)),
                    IconButton(onPressed: _send, icon: const Icon(Icons.send_rounded, color: Palette.accent)),
                  ],
                ),
        ),
      );
}
