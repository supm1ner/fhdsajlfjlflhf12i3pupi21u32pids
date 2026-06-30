import 'package:flutter/foundation.dart';
import 'package:livekit_client/livekit_client.dart';

import '../sunrise/sunrise_client.dart';

typedef ParticipantTile = ({String id, String name, bool isLocal, VideoTrack? track});

/// Wraps a LiveKit [Room] for SFU group calls. Falls back (returns false from [start])
/// when the backend has no LiveKit configured.
class LiveKitController extends ChangeNotifier {
  LiveKitController(this.client);
  final SunriseClient client;

  Room? room;
  bool active = false;
  bool audioOnly = false;
  bool muted = false;
  bool videoOff = false;
  bool screenSharing = false;
  String? error;

  void _onChange() => notifyListeners();

  VideoTrack? _firstVideo(Participant p) {
    for (final pub in p.videoTrackPublications) {
      final t = pub.track;
      if (t is VideoTrack) return t;
    }
    return null;
  }

  List<ParticipantTile> get tiles {
    final r = room;
    if (r == null) return const [];
    final out = <ParticipantTile>[];
    final lp = r.localParticipant;
    if (lp != null) out.add((id: 'local', name: 'You', isLocal: true, track: _firstVideo(lp)));
    for (final p in r.remoteParticipants.values) {
      out.add((id: p.sid, name: p.identity, isLocal: false, track: _firstVideo(p)));
    }
    return out;
  }

  /// Returns true if a LiveKit room was joined; false if LiveKit is not configured.
  Future<bool> start(String roomName, {bool audioOnly = false}) async {
    if (active) return true;
    Map<String, dynamic> tok;
    try {
      tok = await client.fetchLiveKitToken(roomName);
    } on LiveKitNotConfigured {
      return false;
    }
    this.audioOnly = audioOnly;
    final r = Room(roomOptions: const RoomOptions(adaptiveStream: true, dynacast: true));
    room = r;
    r.addListener(_onChange);
    try {
      await r.connect(tok['url'] as String, tok['token'] as String);
      await r.localParticipant?.setMicrophoneEnabled(true);
      if (!audioOnly) await r.localParticipant?.setCameraEnabled(true);
      active = true;
      notifyListeners();
      return true;
    } catch (e) {
      error = e.toString();
      await leave();
      return true; // LiveKit was configured; surface the error rather than fall back
    }
  }

  Future<void> toggleMute() async {
    muted = !muted;
    await room?.localParticipant?.setMicrophoneEnabled(!muted);
    notifyListeners();
  }

  Future<void> toggleVideo() async {
    videoOff = !videoOff;
    await room?.localParticipant?.setCameraEnabled(!videoOff);
    notifyListeners();
  }

  Future<void> toggleScreenShare() async {
    screenSharing = !screenSharing;
    await room?.localParticipant?.setScreenShareEnabled(screenSharing);
    notifyListeners();
  }

  Future<void> leave() async {
    final r = room;
    if (r != null) {
      r.removeListener(_onChange);
      await r.disconnect();
      await r.dispose();
    }
    room = null;
    active = false;
    muted = false;
    videoOff = false;
    screenSharing = false;
    notifyListeners();
  }

  @override
  void dispose() {
    leave();
    super.dispose();
  }
}
