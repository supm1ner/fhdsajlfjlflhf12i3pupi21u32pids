import 'package:flutter/foundation.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart';

import '../sunrise/drafty.dart';
import '../sunrise/sunrise_client.dart';

enum CallStatus { idle, dialing, ringing, incoming, connecting, active }

/// Manages a single 1:1 WebRTC call, mirroring the web client and backend protocol
/// (chat/server/calls.go): invite via head.webrtc='started' + Drafty.videoCall(),
/// offer/answer/ICE via client.videoCall(), events received on client.onInfo (what=='call').
class CallController extends ChangeNotifier {
  CallController(this.client);

  final SunriseClient client;

  final RTCVideoRenderer localRenderer = RTCVideoRenderer();
  final RTCVideoRenderer remoteRenderer = RTCVideoRenderer();
  bool _renderersReady = false;

  bool active = false;
  CallStatus status = CallStatus.idle;
  String? direction; // 'outgoing' | 'incoming'
  bool audioOnly = false;
  String peerName = '';
  String? topic;
  int seq = -1;
  bool muted = false;
  bool videoOff = false;
  bool screenSharing = false;
  String? error;

  RTCPeerConnection? _pc;
  MediaStream? _localStream;
  MediaStreamTrack? _cameraTrack;
  bool _caller = false;
  bool _ready = false;
  final List<RTCIceCandidate> _pendingIce = [];

  static const _ice = {
    'iceServers': [
      {'urls': 'stun:stun.l.google.com:19302'},
    ],
  };

  Future<void> _ensureRenderers() async {
    if (_renderersReady) return;
    await localRenderer.initialize();
    await remoteRenderer.initialize();
    _renderersReady = true;
  }

  Map<String, dynamic> get _constraints => {
        'audio': true,
        'video': audioOnly ? false : {'facingMode': 'user'},
      };

  Future<void> _getLocalMedia() async {
    if (_localStream != null) return;
    _localStream = await navigator.mediaDevices.getUserMedia(_constraints);
    localRenderer.srcObject = _localStream;
  }

  Future<RTCPeerConnection> _createPc() async {
    final pc = await createPeerConnection(_ice);
    pc.onIceCandidate = (c) {
      if (c.candidate != null && topic != null) {
        client.videoCall(topic!, 'ice-candidate', seq, c.toMap());
      }
    };
    pc.onTrack = (e) {
      if (e.streams.isNotEmpty) {
        remoteRenderer.srcObject = e.streams[0];
        status = CallStatus.active;
        notifyListeners();
      }
    };
    pc.onConnectionState = (s) {
      if (s == RTCPeerConnectionState.RTCPeerConnectionStateFailed) hangup();
    };
    return pc;
  }

  // --- Public actions ----------------------------------------------------

  Future<void> startCall(String topicName, String name, {bool audioOnly = false}) async {
    if (active) return;
    active = true;
    direction = 'outgoing';
    status = CallStatus.dialing;
    this.audioOnly = audioOnly;
    peerName = name;
    topic = topicName;
    seq = -1;
    _caller = true;
    error = null;
    notifyListeners();
    try {
      await _ensureRenderers();
      await client.subscribe(topicName);
      await _getLocalMedia();
      final ctrl = await client.publish(
        topicName,
        Drafty.videoCall(audioOnly),
        head: {'webrtc': 'started', 'aonly': audioOnly},
      );
      seq = ((ctrl['params'] as Map?)?['seq'] as num?)?.toInt() ?? -1;
      notifyListeners();
    } catch (e) {
      error = e.toString();
      hangup();
    }
  }

  Future<void> handleIncoming(String topicName, int callSeq, bool audioOnly, String name) async {
    if (active) {
      client.videoCall(topicName, 'hang-up', callSeq);
      return;
    }
    active = true;
    direction = 'incoming';
    status = CallStatus.incoming;
    this.audioOnly = audioOnly;
    peerName = name;
    topic = topicName;
    seq = callSeq;
    _caller = false;
    notifyListeners();
    client.videoCall(topicName, 'ringing', callSeq);
  }

  Future<void> accept() async {
    if (!active || direction != 'incoming') return;
    status = CallStatus.connecting;
    notifyListeners();
    try {
      await _ensureRenderers();
      await client.subscribe(topic!);
      await _getLocalMedia();
      client.videoCall(topic!, 'accept', seq);
    } catch (e) {
      error = e.toString();
      hangup();
    }
  }

  void decline() {
    if (!active) return;
    if (topic != null) client.videoCall(topic!, 'hang-up', seq);
    _teardown();
  }

  void hangup() {
    if (!active) return;
    if (topic != null) client.videoCall(topic!, 'hang-up', seq);
    _teardown();
  }

  void toggleMute() {
    final tracks = _localStream?.getAudioTracks();
    if (tracks == null || tracks.isEmpty) return;
    final track = tracks.first;
    track.enabled = !track.enabled;
    muted = !track.enabled;
    notifyListeners();
  }

  void toggleVideo() {
    final tracks = _localStream?.getVideoTracks();
    if (tracks == null || tracks.isEmpty) return;
    final track = tracks.first;
    track.enabled = !track.enabled;
    videoOff = !track.enabled;
    notifyListeners();
  }

  /// Swap the outgoing video track between the camera and the screen.
  Future<void> toggleScreenShare() async {
    final pc = _pc;
    if (pc == null) return;
    final senders = await pc.getSenders();
    final sender = senders.where((s) => s.track?.kind == 'video').cast<RTCRtpSender?>().firstWhere((_) => true, orElse: () => null);
    if (sender == null) return;
    try {
      if (!screenSharing) {
        final display = await navigator.mediaDevices.getDisplayMedia({'video': true, 'audio': false});
        final screenTrack = display.getVideoTracks().first;
        _cameraTrack = sender.track;
        await sender.replaceTrack(screenTrack);
        screenSharing = true;
      } else {
        if (_cameraTrack != null) await sender.replaceTrack(_cameraTrack);
        _cameraTrack = null;
        screenSharing = false;
      }
      notifyListeners();
    } catch (e) {
      error = e.toString();
      notifyListeners();
    }
  }

  // --- Signaling ---------------------------------------------------------

  /// Routes a {info what:'call'} event from the server.
  Future<void> onSignal(Map<String, dynamic> info) async {
    if (info['what'] != 'call') return;
    final event = info['event'] as String?;
    final s = (info['seq'] as num?)?.toInt();
    if (s != null && seq != -1 && s != seq) return;
    final payload = info['payload'];
    try {
      switch (event) {
        case 'ringing':
          if (_caller) {
            status = CallStatus.ringing;
            notifyListeners();
          }
          break;
        case 'accept':
          if (_caller) await _beginAsCaller();
          break;
        case 'offer':
          if (!_caller) await _handleOffer(payload as Map<String, dynamic>);
          break;
        case 'answer':
          if (_caller && _pc != null) {
            await _pc!.setRemoteDescription(RTCSessionDescription(payload['sdp'], payload['type']));
            _ready = true;
            _drainIce();
          }
          break;
        case 'ice-candidate':
          final cand = RTCIceCandidate(payload['candidate'], payload['sdpMid'], payload['sdpMLineIndex']);
          if (_pc != null && _ready) {
            await _pc!.addCandidate(cand);
          } else {
            _pendingIce.add(cand);
          }
          break;
        case 'hang-up':
          _teardown();
          break;
      }
    } catch (e) {
      error = e.toString();
      notifyListeners();
    }
  }

  Future<void> _beginAsCaller() async {
    if (_pc != null) return;
    status = CallStatus.connecting;
    notifyListeners();
    _pc = await _createPc();
    await _getLocalMedia();
    _localStream!.getTracks().forEach((t) => _pc!.addTrack(t, _localStream!));
    final offer = await _pc!.createOffer();
    await _pc!.setLocalDescription(offer);
    client.videoCall(topic!, 'offer', seq, offer.toMap());
  }

  Future<void> _handleOffer(Map<String, dynamic> payload) async {
    _pc ??= await _createPc();
    await _pc!.setRemoteDescription(RTCSessionDescription(payload['sdp'], payload['type']));
    await _getLocalMedia();
    _localStream!.getTracks().forEach((t) => _pc!.addTrack(t, _localStream!));
    final answer = await _pc!.createAnswer();
    await _pc!.setLocalDescription(answer);
    client.videoCall(topic!, 'answer', seq, answer.toMap());
    _ready = true;
    _drainIce();
  }

  void _drainIce() {
    for (final c in _pendingIce) {
      _pc?.addCandidate(c);
    }
    _pendingIce.clear();
  }

  void _teardown() {
    _localStream?.getTracks().forEach((t) => t.stop());
    _localStream?.dispose();
    _localStream = null;
    localRenderer.srcObject = null;
    remoteRenderer.srcObject = null;
    _pc?.close();
    _pc = null;
    _ready = false;
    _pendingIce.clear();
    active = false;
    status = CallStatus.idle;
    direction = null;
    topic = null;
    seq = -1;
    muted = false;
    videoOff = false;
    screenSharing = false;
    _cameraTrack = null;
    _caller = false;
    notifyListeners();
  }

  @override
  void dispose() {
    _teardown();
    localRenderer.dispose();
    remoteRenderer.dispose();
    super.dispose();
  }
}
