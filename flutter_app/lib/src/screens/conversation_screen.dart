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
    setState(() => _recordingVoice = true);
  }

  @override
  void dispose() {
    _input.dispose();
    _scroll.dispose();
    _recorder.dispose();
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
        _header(s),
        Expanded(
          child: s.messages.isEmpty
              ? const Center(child: Text('No messages yet. Say hello!', style: TextStyle(color: Palette.textSecondary)))
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
                    Text(s.peerTyping ? 'typing…' : 'online',
                        style: const TextStyle(color: Palette.textTertiary, fontSize: 11)),
                  ],
                ),
              ),
              IconButton(
                  onPressed: () => s.startCall(audioOnly: true),
                  icon: const Icon(Icons.call, color: Palette.textSecondary)),
              IconButton(
                  onPressed: () => s.startCall(audioOnly: false),
                  icon: const Icon(Icons.videocam, color: Palette.textSecondary)),
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
                  const Expanded(child: Text('Recording voice…', style: TextStyle(color: Palette.textSecondary))),
                  IconButton(onPressed: _toggleVoice, icon: const Icon(Icons.send_rounded, color: Palette.accent)),
                ])
              : Row(
                  children: [
                    IconButton(onPressed: _attachImage, icon: const Icon(Icons.image_outlined, color: Palette.textSecondary)),
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
