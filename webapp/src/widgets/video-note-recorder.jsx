// Video note (кружок) recorder widget: records a short, square, round-framed
// video message like Telegram's video notes.

import React from 'react';
import { defineMessages, injectIntl } from 'react-intl';

// Workaround for https://bugs.chromium.org/p/chromium/issues/detail?id=642012
// It adds duration and SeekHead to the webm record.
import fixWebmDuration from 'webm-duration-fix';

import { secondsToTime } from '../lib/strformat';
import { KEYPRESS_DELAY, MIN_DURATION } from '../config.js';

// Square capture dimension (both width and height). Square marks it as a round video note.
const VIDEO_NOTE_DIM = 240;
// Video notes are meant to be short. Telegram caps them at 60 seconds.
const VIDEO_NOTE_MAX_DURATION = 60_000;

// Preferred recording formats in order of preference (Safari supports only mp4).
const VIDEO_MIME_TYPES = [
  'video/webm;codecs=vp8,opus',
  'video/webm;codecs=vp9,opus',
  'video/webm',
  'video/mp4',
  ''
];

const messages = defineMessages({
  icon_title_delete: {
    id: 'icon_title_delete_video_note',
    defaultMessage: 'Cancel recording',
    description: 'Icon tool tip for cancelling a video note recording'
  },
  icon_title_send: {
    id: 'icon_title_send',
    defaultMessage: 'Send message',
    description: 'Icon tool tip for sending a message'
  },
  failed_to_init_video: {
    id: 'failed_to_init_video',
    defaultMessage: 'Failed to initialize video recording',
    description: 'Error message when the camera is not available'
  }
});

class VideoNoteRecorder extends React.PureComponent {
  constructor(props) {
    super(props);

    this.state = {
      recording: false,
      duration: '0:00'
    };

    this.initMediaRecording = this.initMediaRecording.bind(this);
    this.tick = this.tick.bind(this);
    this.getRecording = this.getRecording.bind(this);
    this.grabPreview = this.grabPreview.bind(this);
    this.cleanUp = this.cleanUp.bind(this);
    this.handleDelete = this.handleDelete.bind(this);
    this.handleDone = this.handleDone.bind(this);

    this.videoRef = React.createRef();

    this.durationMillis = 0;
    this.startedOn = null;
    // Timestamp for sending "recording" notifications.
    this.recordingTimestamp = 0;
    // Set to true once the recording is finished to avoid double-send.
    this.finished = false;
  }

  componentDidMount() {
    this.stream = null;
    this.mediaRecorder = null;
    this.videoChunks = [];

    try {
      navigator.mediaDevices.getUserMedia({
        audio: true,
        video: {
          width: {ideal: VIDEO_NOTE_DIM},
          height: {ideal: VIDEO_NOTE_DIM},
          facingMode: 'user',
          aspectRatio: {ideal: 1}
        }
      }).then(this.initMediaRecording, this.props.onError);
    } catch (err) {
      this.props.onError(err);
    }
  }

  componentWillUnmount() {
    this.startedOn = null;
    if (this.stream) {
      this.cleanUp();
    }
  }

  initMediaRecording(stream) {
    this.stream = stream;

    // Show the live camera feed.
    if (this.videoRef.current) {
      this.videoRef.current.srcObject = stream;
      this.videoRef.current.play().catch(_ => { /* autoplay may be blocked; ignore */ });
    }

    let mimeType = null;
    VIDEO_MIME_TYPES.some(mt => {
      if (MediaRecorder.isTypeSupported(mt)) {
        mimeType = mt;
        return true;
      }
      return false;
    });

    if (mimeType == null) {
      console.warn('MediaRecorder failed to initialize: no supported video formats');
      this.props.onError(this.props.intl.formatMessage(messages.failed_to_init_video));
      this.cleanUp();
      return;
    }

    try {
      this.mediaRecorder = new MediaRecorder(stream, {
        mimeType: mimeType,
        videoBitsPerSecond: 1_000_000,
        audioBitsPerSecond: 24_000
      });
    } catch (err) {
      console.warn('MediaRecorder failed to initialize:', err);
      this.props.onError(this.props.intl.formatMessage(messages.failed_to_init_video));
      this.cleanUp();
      return;
    }

    this.mediaRecorder.ondataavailable = (e) => {
      if (e.data && e.data.size > 0) {
        this.videoChunks.push(e.data);
      }
    };

    this.mediaRecorder.onstop = _ => {
      // Grab a preview frame before the stream is stopped.
      const preview = this.grabPreview();
      if (!this.finished || this.durationMillis <= MIN_DURATION) {
        // Cancelled or too short to send.
        this.props.onDeleted();
        this.cleanUp();
        return;
      }
      this.getRecording(this.mediaRecorder.mimeType)
        .then(videoBlob => {
          this.props.onFinished(videoBlob, preview, {
            width: VIDEO_NOTE_DIM,
            height: VIDEO_NOTE_DIM,
            duration: this.durationMillis,
            mime: videoBlob.type,
            name: 'video-note.webm'
          });
        })
        .catch(err => this.props.onError(err.message || err))
        .finally(_ => this.cleanUp());
    };

    this.durationMillis = 0;
    this.startedOn = Date.now();
    this.mediaRecorder.start();
    this.setState({recording: true});
    this.tick();

    this.props.onRecordingProgress();
    this.recordingTimestamp = this.startedOn;
  }

  // Update the duration timer and auto-stop when the limit is reached.
  tick() {
    if (!this.startedOn) {
      return;
    }
    const duration = this.durationMillis + (Date.now() - this.startedOn);
    this.setState({duration: secondsToTime(duration / 1000)});

    // Notify the server that recording is in progress.
    const now = Date.now();
    if (now - this.recordingTimestamp > KEYPRESS_DELAY) {
      this.props.onRecordingProgress();
      this.recordingTimestamp = now;
    }

    if (duration >= VIDEO_NOTE_MAX_DURATION) {
      // Reached the maximum length: finish automatically.
      this.durationMillis = duration;
      this.finished = true;
      this.startedOn = null;
      if (this.mediaRecorder && this.mediaRecorder.state != 'inactive') {
        this.mediaRecorder.stop();
      }
      return;
    }

    window.requestAnimationFrame(this.tick);
  }

  // Capture the current video frame as a square JPEG preview blob.
  grabPreview() {
    const video = this.videoRef.current;
    if (!video || !video.videoWidth || !video.videoHeight) {
      return null;
    }
    const canvas = document.createElement('canvas');
    canvas.width = VIDEO_NOTE_DIM;
    canvas.height = VIDEO_NOTE_DIM;
    const ctx = canvas.getContext('2d');
    // Center-crop the (possibly non-square) camera frame into a square.
    const side = Math.min(video.videoWidth, video.videoHeight);
    const sx = (video.videoWidth - side) / 2;
    const sy = (video.videoHeight - side) / 2;
    ctx.drawImage(video, sx, sy, side, side, 0, 0, VIDEO_NOTE_DIM, VIDEO_NOTE_DIM);
    return canvas.toDataURL('image/jpeg', 0.7);
  }

  // Assemble recorded chunks into a single video blob.
  getRecording(mimeType) {
    mimeType = mimeType || VIDEO_MIME_TYPES[0];
    const blob = new Blob(this.videoChunks, {type: mimeType});
    // Fix Chrome's WebM duration bug for webm recordings.
    if (mimeType.startsWith('video/webm')) {
      return fixWebmDuration(blob, mimeType).catch(_ => blob);
    }
    return Promise.resolve(blob);
  }

  handleDelete(e) {
    e.preventDefault();
    this.finished = false;
    this.durationMillis = 0;
    this.startedOn = null;
    if (this.mediaRecorder && this.mediaRecorder.state != 'inactive') {
      this.mediaRecorder.stop();
    } else {
      this.props.onDeleted();
      this.cleanUp();
    }
  }

  handleDone(e) {
    e.preventDefault();
    if (this.startedOn) {
      this.durationMillis += Date.now() - this.startedOn;
      this.startedOn = null;
    }
    this.finished = true;
    this.setState({recording: false});
    if (this.mediaRecorder && this.mediaRecorder.state != 'inactive') {
      this.mediaRecorder.stop();
    }
  }

  cleanUp() {
    this.startedOn = null;
    if (this.videoRef.current) {
      this.videoRef.current.srcObject = null;
    }
    if (this.stream) {
      this.stream.getTracks().forEach(track => track.stop());
      this.stream = null;
    }
  }

  render() {
    const {formatMessage} = this.props.intl;
    return (
      <div className="video-note-recorder">
        <a href="#" onClick={this.handleDelete} title={formatMessage(messages.icon_title_delete)}>
          <i className="material-icons gray">delete_outline</i>
        </a>
        <div className="video-note-preview">
          <video ref={this.videoRef} muted playsInline />
          <span className="rec-dot" />
        </div>
        <div className="duration">{this.state.duration}</div>
        <a href="#" onClick={this.handleDone} title={formatMessage(messages.icon_title_send)}>
          <i className="material-icons">send</i>
        </a>
      </div>
    );
  }
}

export default injectIntl(VideoNoteRecorder);
