import 'package:flutter/material.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart';

import '../state/call_controller.dart';
import '../theme/glass_theme.dart';

/// Immersive (dark) active-call screen: remote video full-bleed, local PIP, controls.
class CallScreen extends StatelessWidget {
  const CallScreen({super.key, required this.call});
  final CallController call;

  String get _statusText {
    switch (call.status) {
      case CallStatus.dialing:
        return 'Calling…';
      case CallStatus.ringing:
        return 'Ringing…';
      case CallStatus.connecting:
        return 'Connecting…';
      case CallStatus.active:
        return 'Connected';
      default:
        return '';
    }
  }

  @override
  Widget build(BuildContext context) {
    return Material(
      color: Palette.callBg,
      child: Stack(
        fit: StackFit.expand,
        children: [
          if (!call.audioOnly)
            RTCVideoView(call.remoteRenderer, objectFit: RTCVideoViewObjectFit.RTCVideoViewObjectFitCover)
          else
            _audioCard(),
          if (!call.audioOnly)
            Positioned(
              right: 16,
              bottom: 110,
              width: 120,
              height: 170,
              child: ClipRRect(
                borderRadius: BorderRadius.circular(14),
                child: RTCVideoView(call.localRenderer, mirror: true,
                    objectFit: RTCVideoViewObjectFit.RTCVideoViewObjectFitCover),
              ),
            ),
          Positioned(
            top: 48,
            left: 0,
            right: 0,
            child: Column(children: [
              Text(call.peerName,
                  style: const TextStyle(color: Colors.white, fontSize: 20, fontWeight: FontWeight.w600)),
              const SizedBox(height: 4),
              Text(_statusText, style: const TextStyle(color: Colors.white70, fontSize: 13)),
              if (call.error != null)
                Padding(
                  padding: const EdgeInsets.only(top: 6),
                  child: Text(call.error!, style: const TextStyle(color: Palette.danger, fontSize: 12)),
                ),
            ]),
          ),
          Positioned(
            bottom: 36,
            left: 0,
            right: 0,
            child: Row(mainAxisAlignment: MainAxisAlignment.center, children: [
              _circle(call.muted ? Icons.mic_off : Icons.mic, call.toggleMute, const Color(0x33FFFFFF)),
              const SizedBox(width: 18),
              if (!call.audioOnly)
                _circle(call.videoOff ? Icons.videocam_off : Icons.videocam, call.toggleVideo, const Color(0x33FFFFFF)),
              if (!call.audioOnly) const SizedBox(width: 18),
              _circle(Icons.call_end, call.hangup, Palette.danger),
            ]),
          ),
        ],
      ),
    );
  }

  Widget _audioCard() => Center(
        child: Column(mainAxisSize: MainAxisSize.min, children: [
          CircleAvatar(
            radius: 60,
            backgroundColor: Palette.accent,
            child: Text(call.peerName.isNotEmpty ? call.peerName[0].toUpperCase() : '?',
                style: const TextStyle(color: Colors.white, fontSize: 44)),
          ),
        ]),
      );

  Widget _circle(IconData icon, VoidCallback onTap, Color bg) {
    return InkResponse(
      onTap: onTap,
      child: Container(
        width: 60,
        height: 60,
        decoration: BoxDecoration(color: bg, shape: BoxShape.circle),
        child: Icon(icon, color: Colors.white, size: 26),
      ),
    );
  }
}
