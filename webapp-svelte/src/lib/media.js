// Media helpers: upload attachments and compose Drafty messages for images, files,
// voice messages and round video notes ("кружки"). Uploads complete before the message
// is published (the ref is embedded directly), which keeps the send path deterministic.

import { getClient, getUploader, Drafty } from './tinode.js';

// blobToBase64 returns the base64 payload (without the data: prefix) and the mime type.
export function blobToBase64(blob) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const res = reader.result || '';
      const comma = res.indexOf(',');
      resolve({ mime: blob.type, bits: comma >= 0 ? res.slice(comma + 1) : res });
    };
    reader.onerror = reject;
    reader.readAsDataURL(blob);
  });
}

// imageSize resolves the natural dimensions of an image blob.
function imageSize(blob) {
  return new Promise((resolve) => {
    const url = URL.createObjectURL(blob);
    const img = new Image();
    img.onload = () => { resolve({ width: img.naturalWidth, height: img.naturalHeight }); URL.revokeObjectURL(url); };
    img.onerror = () => { resolve({ width: 0, height: 0 }); URL.revokeObjectURL(url); };
    img.src = url;
  });
}

async function topicFor(topicName) {
  const topic = getClient().getTopic(topicName);
  if (!topic.isSubscribed()) await topic.subscribe();
  return topic;
}

async function upload(blob, name) {
  const uploader = getUploader();
  // Returns the server file ref, e.g. "/v0/file/s/<id>".
  return await uploader.upload(blob, name);
}

// sendText publishes a rich-text message (links/formatting auto-detected).
export async function sendText(topicName, text) {
  const topic = await topicFor(topicName);
  return topic.publish(Drafty.parse(text));
}

// sendImage uploads and sends an image attachment.
export async function sendImage(topicName, file) {
  const topic = await topicFor(topicName);
  const { width, height } = await imageSize(file);
  const ref = await upload(file, file.name);
  const content = Drafty.insertImage(null, 0, {
    mime: file.type, refurl: ref, width, height, filename: file.name, size: file.size,
  });
  return topic.publish(content);
}

// sendFile uploads and sends a generic file attachment.
export async function sendFile(topicName, file) {
  const topic = await topicFor(topicName);
  const ref = await upload(file, file.name);
  const content = Drafty.attachFile(null, {
    mime: file.type || 'application/octet-stream', refurl: ref, filename: file.name, size: file.size,
  });
  return topic.publish(content);
}

// sendVoice uploads and sends a voice message.
export async function sendVoice(topicName, blob, durationMs) {
  const topic = await topicFor(topicName);
  const ref = await upload(blob, 'voice-message.webm');
  const content = Drafty.insertAudio(null, 0, {
    mime: blob.type, refurl: ref, duration: durationMs | 0, filename: 'voice-message.webm', size: blob.size,
  });
  return topic.publish(content);
}

// sendVideoNote uploads and sends a round video note ("кружок"): a square video.
export async function sendVideoNote(topicName, blob, durationMs, side = 240) {
  const topic = await topicFor(topicName);
  const ref = await upload(blob, 'video-note.webm');
  const content = Drafty.insertVideo(null, 0, {
    mime: blob.type, refurl: ref, width: side, height: side,
    duration: durationMs | 0, filename: 'video-note.webm', size: blob.size,
  });
  return topic.publish(content);
}

// pickMimeType returns a supported MediaRecorder mime type for the given kind.
export function pickMimeType(kind) {
  const candidates = kind === 'audio'
    ? ['audio/webm;codecs=opus', 'audio/webm', 'audio/mp4']
    : ['video/webm;codecs=vp9,opus', 'video/webm;codecs=vp8,opus', 'video/webm', 'video/mp4'];
  for (const c of candidates) {
    if (typeof MediaRecorder !== 'undefined' && MediaRecorder.isTypeSupported?.(c)) return c;
  }
  return kind === 'audio' ? 'audio/webm' : 'video/webm';
}
