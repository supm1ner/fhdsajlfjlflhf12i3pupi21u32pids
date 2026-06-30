import 'package:flutter/material.dart';
import 'package:livekit_client/livekit_client.dart';

import '../state/livekit_room.dart';
import '../theme/glass_theme.dart';

/// Immersive LiveKit group-call grid.
class LiveKitRoomScreen extends StatelessWidget {
  const LiveKitRoomScreen({super.key, required this.controller});
  final LiveKitController controller;

  @override
  Widget build(BuildContext context) {
    final tiles = controller.tiles;
    final cols = tiles.length <= 1 ? 1 : (tiles.length <= 4 ? 2 : 3);
    return Material(
      color: Palette.callBg,
      child: Column(
        children: [
          const SizedBox(height: 16),
          Text('Group call · ${tiles.length}',
              style: const TextStyle(color: Colors.white, fontWeight: FontWeight.w600)),
          if (controller.error != null)
            Padding(
              padding: const EdgeInsets.all(8),
              child: Text(controller.error!, style: const TextStyle(color: Palette.danger, fontSize: 12)),
            ),
          Expanded(
            child: Padding(
              padding: const EdgeInsets.all(12),
              child: GridView.count(
                crossAxisCount: cols,
                mainAxisSpacing: 10,
                crossAxisSpacing: 10,
                childAspectRatio: 4 / 3,
                children: [
                  for (final t in tiles)
                    ClipRRect(
                      borderRadius: BorderRadius.circular(14),
                      child: Stack(
                        fit: StackFit.expand,
                        children: [
                          if (t.track != null)
                            VideoTrackRenderer(t.track!)
                          else
                            Container(
                              color: Palette.accent,
                              alignment: Alignment.center,
                              child: Text(t.name.isNotEmpty ? t.name[0].toUpperCase() : '?',
                                  style: const TextStyle(color: Colors.white, fontSize: 32)),
                            ),
                          Positioned(
                            left: 8,
                            bottom: 8,
                            child: Container(
                              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                              decoration: BoxDecoration(color: Colors.black54, borderRadius: BorderRadius.circular(8)),
                              child: Text(t.name, style: const TextStyle(color: Colors.white, fontSize: 12)),
                            ),
                          ),
                        ],
                      ),
                    ),
                ],
              ),
            ),
          ),
          Padding(
            padding: const EdgeInsets.all(20),
            child: Row(mainAxisAlignment: MainAxisAlignment.center, children: [
              _btn(controller.muted ? Icons.mic_off : Icons.mic, controller.toggleMute, const Color(0x33FFFFFF)),
              const SizedBox(width: 16),
              if (!controller.audioOnly)
                _btn(controller.videoOff ? Icons.videocam_off : Icons.videocam, controller.toggleVideo, const Color(0x33FFFFFF)),
              if (!controller.audioOnly) const SizedBox(width: 16),
              if (!controller.audioOnly)
                _btn(Icons.screen_share, controller.toggleScreenShare,
                    controller.screenSharing ? Palette.accent : const Color(0x33FFFFFF)),
              if (!controller.audioOnly) const SizedBox(width: 16),
              _btn(Icons.call_end, controller.leave, Palette.danger),
            ]),
          ),
        ],
      ),
    );
  }

  Widget _btn(IconData icon, VoidCallback onTap, Color bg) => InkResponse(
        onTap: onTap,
        child: Container(
          width: 56,
          height: 56,
          decoration: BoxDecoration(color: bg, shape: BoxShape.circle),
          child: Icon(icon, color: Colors.white, size: 24),
        ),
      );
}
