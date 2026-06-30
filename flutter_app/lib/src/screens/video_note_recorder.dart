import 'dart:async';

import 'package:camera/camera.dart';
import 'package:flutter/material.dart';
import 'package:permission_handler/permission_handler.dart';

import '../theme/glass_theme.dart';

/// Result of recording a round video note.
class VideoNoteResult {
  VideoNoteResult(this.bytes, this.durationMs);
  final List<int> bytes;
  final int durationMs;
}

/// Records a short round video note ("кружок"). Pops with a [VideoNoteResult] or null.
class VideoNoteRecorderScreen extends StatefulWidget {
  const VideoNoteRecorderScreen({super.key});

  @override
  State<VideoNoteRecorderScreen> createState() => _VideoNoteRecorderScreenState();
}

class _VideoNoteRecorderScreenState extends State<VideoNoteRecorderScreen> {
  CameraController? _controller;
  bool _recording = false;
  String? _error;
  final _stopwatch = Stopwatch();
  Timer? _ticker;
  int _elapsedMs = 0;
  static const _maxMs = 60000;

  @override
  void initState() {
    super.initState();
    _init();
  }

  Future<void> _init() async {
    try {
      final cam = await Permission.camera.request();
      final mic = await Permission.microphone.request();
      if (!cam.isGranted || !mic.isGranted) {
        setState(() => _error = 'Camera/microphone permission denied');
        return;
      }
      final cameras = await availableCameras();
      final front = cameras.firstWhere(
        (c) => c.lensDirection == CameraLensDirection.front,
        orElse: () => cameras.first,
      );
      final controller = CameraController(front, ResolutionPreset.medium, enableAudio: true);
      await controller.initialize();
      if (!mounted) return;
      setState(() => _controller = controller);
    } catch (e) {
      setState(() => _error = '$e');
    }
  }

  Future<void> _start() async {
    final c = _controller;
    if (c == null || _recording) return;
    await c.startVideoRecording();
    _stopwatch
      ..reset()
      ..start();
    _ticker = Timer.periodic(const Duration(milliseconds: 100), (_) {
      setState(() => _elapsedMs = _stopwatch.elapsedMilliseconds);
      if (_elapsedMs >= _maxMs) _stop();
    });
    setState(() => _recording = true);
  }

  Future<void> _stop() async {
    final c = _controller;
    if (c == null || !_recording) return;
    _ticker?.cancel();
    _stopwatch.stop();
    final file = await c.stopVideoRecording();
    final bytes = await file.readAsBytes();
    if (!mounted) return;
    Navigator.of(context).pop(VideoNoteResult(bytes, _stopwatch.elapsedMilliseconds));
  }

  @override
  void dispose() {
    _ticker?.cancel();
    _controller?.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Palette.callBg,
      body: Center(
        child: _error != null
            ? Padding(
                padding: const EdgeInsets.all(24),
                child: Text(_error!, style: const TextStyle(color: Colors.white), textAlign: TextAlign.center),
              )
            : Column(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  Container(
                    width: 260,
                    height: 260,
                    decoration: BoxDecoration(
                      shape: BoxShape.circle,
                      border: Border.all(color: _recording ? Palette.danger : Colors.white24, width: 4),
                    ),
                    child: ClipOval(
                      child: _controller != null && _controller!.value.isInitialized
                          ? FittedBox(
                              fit: BoxFit.cover,
                              child: SizedBox(
                                width: _controller!.value.previewSize?.height ?? 260,
                                height: _controller!.value.previewSize?.width ?? 260,
                                child: CameraPreview(_controller!),
                              ),
                            )
                          : const ColoredBox(color: Colors.black),
                    ),
                  ),
                  const SizedBox(height: 18),
                  Text(_recording ? 'Recording… ${(_elapsedMs / 1000).toStringAsFixed(1)}s' : 'Record a round video note',
                      style: const TextStyle(color: Colors.white, fontSize: 14)),
                  const SizedBox(height: 24),
                  Row(mainAxisAlignment: MainAxisAlignment.center, children: [
                    _btn(Icons.close, () => Navigator.of(context).pop(), const Color(0x33FFFFFF)),
                    const SizedBox(width: 28),
                    _recording
                        ? _btn(Icons.check, _stop, Palette.accent)
                        : _btn(Icons.fiber_manual_record, _start, Palette.danger),
                  ]),
                ],
              ),
      ),
    );
  }

  Widget _btn(IconData icon, VoidCallback onTap, Color bg) => InkResponse(
        onTap: onTap,
        child: Container(
          width: 60,
          height: 60,
          decoration: BoxDecoration(color: bg, shape: BoxShape.circle),
          child: Icon(icon, color: Colors.white, size: 26),
        ),
      );
}
