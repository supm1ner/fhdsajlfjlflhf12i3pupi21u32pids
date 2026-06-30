import 'package:flutter/material.dart';

import '../state/call_controller.dart';
import '../theme/glass_theme.dart';

/// Incoming-call prompt overlay.
class IncomingCallView extends StatelessWidget {
  const IncomingCallView({super.key, required this.call});
  final CallController call;

  @override
  Widget build(BuildContext context) {
    return Material(
      color: Colors.black54,
      child: Center(
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 36, vertical: 32),
          decoration: BoxDecoration(
            color: Palette.bg,
            borderRadius: BorderRadius.circular(24),
            boxShadow: Palette.cardShadow,
          ),
          child: Column(mainAxisSize: MainAxisSize.min, children: [
            CircleAvatar(
              radius: 44,
              backgroundColor: Palette.accent,
              child: Text(call.peerName.isNotEmpty ? call.peerName[0].toUpperCase() : '?',
                  style: const TextStyle(color: Colors.white, fontSize: 34)),
            ),
            const SizedBox(height: 14),
            Text(call.peerName,
                style: const TextStyle(color: Palette.textPrimary, fontSize: 20, fontWeight: FontWeight.w600)),
            const SizedBox(height: 4),
            Text('Incoming ${call.audioOnly ? 'voice' : 'video'} call…',
                style: const TextStyle(color: Palette.textSecondary, fontSize: 13)),
            const SizedBox(height: 26),
            Row(mainAxisAlignment: MainAxisAlignment.center, children: [
              _circle(Icons.call_end, call.decline, Palette.danger),
              const SizedBox(width: 48),
              _circle(Icons.call, call.accept, Palette.success),
            ]),
          ]),
        ),
      ),
    );
  }

  Widget _circle(IconData icon, VoidCallback onTap, Color bg) => InkResponse(
        onTap: onTap,
        child: Container(
          width: 62,
          height: 62,
          decoration: BoxDecoration(color: bg, shape: BoxShape.circle),
          child: Icon(icon, color: Colors.white, size: 28),
        ),
      );
}
