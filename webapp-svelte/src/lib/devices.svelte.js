// Audio/video device management for calls (desktop/PC): enumerate input/output devices,
// remember the user's choice, build getUserMedia constraints, and route audio output.

const STORE_KEY = 'sunrise_devices';

let _s = $state({
  audioInput: '',   // microphone deviceId
  audioOutput: '',  // speaker deviceId (setSinkId)
  videoInput: '',   // camera deviceId
  noiseSuppression: true,
  echoCancellation: true,
  autoGainControl: true,
  devices: { audioinput: [], audiooutput: [], videoinput: [] },
});

function load() {
  try {
    const raw = localStorage.getItem(STORE_KEY);
    if (raw) Object.assign(_s, JSON.parse(raw));
  } catch { /* ignore */ }
}
load();

function persist() {
  const { devices, ...prefs } = _s;
  localStorage.setItem(STORE_KEY, JSON.stringify(prefs));
}

export const deviceState = {
  get audioInput() { return _s.audioInput; },
  get audioOutput() { return _s.audioOutput; },
  get videoInput() { return _s.videoInput; },
  get noiseSuppression() { return _s.noiseSuppression; },
  get echoCancellation() { return _s.echoCancellation; },
  get autoGainControl() { return _s.autoGainControl; },
  get devices() { return _s.devices; },
};

// refresh enumerates available media devices (labels require a prior permission grant).
export async function refreshDevices() {
  if (!navigator.mediaDevices?.enumerateDevices) return;
  const list = await navigator.mediaDevices.enumerateDevices();
  const grouped = { audioinput: [], audiooutput: [], videoinput: [] };
  for (const d of list) {
    if (grouped[d.kind]) grouped[d.kind].push({ deviceId: d.deviceId, label: d.label || d.kind });
  }
  _s.devices = grouped;
}

export function setAudioInput(id) { _s.audioInput = id; persist(); }
export function setAudioOutput(id) { _s.audioOutput = id; persist(); }
export function setVideoInput(id) { _s.videoInput = id; persist(); }
export function setProcessing({ noiseSuppression, echoCancellation, autoGainControl }) {
  if (noiseSuppression !== undefined) _s.noiseSuppression = noiseSuppression;
  if (echoCancellation !== undefined) _s.echoCancellation = echoCancellation;
  if (autoGainControl !== undefined) _s.autoGainControl = autoGainControl;
  persist();
}

// constraints builds getUserMedia constraints honoring the selected devices and processing.
export function constraints(audioOnly) {
  const audio = {
    echoCancellation: _s.echoCancellation,
    noiseSuppression: _s.noiseSuppression,
    autoGainControl: _s.autoGainControl,
  };
  if (_s.audioInput) audio.deviceId = { exact: _s.audioInput };
  const video = audioOnly
    ? false
    : (_s.videoInput ? { deviceId: { exact: _s.videoInput } } : { width: 640, height: 480 });
  return { audio, video };
}

// applySink routes a media element's audio output to the selected speaker (Chromium/desktop).
export async function applySink(mediaEl) {
  if (_s.audioOutput && typeof mediaEl?.setSinkId === 'function') {
    try { await mediaEl.setSinkId(_s.audioOutput); } catch { /* not supported */ }
  }
}
