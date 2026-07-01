"use strict";
(self["webpackChunksunrise_webapp"] = self["webpackChunksunrise_webapp"] || []).push([["src_widgets_video-note-recorder_jsx"],{

/***/ "./src/widgets/video-note-recorder.jsx":
/*!*********************************************!*\
  !*** ./src/widgets/video-note-recorder.jsx ***!
  \*********************************************/
/***/ (function(__unused_webpack_module, __webpack_exports__, __webpack_require__) {

__webpack_require__.r(__webpack_exports__);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(/*! react */ "react");
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var react_intl__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(/*! react-intl */ "react-intl");
/* harmony import */ var react_intl__WEBPACK_IMPORTED_MODULE_1___default = /*#__PURE__*/__webpack_require__.n(react_intl__WEBPACK_IMPORTED_MODULE_1__);
/* harmony import */ var webm_duration_fix__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(/*! webm-duration-fix */ "./node_modules/webm-duration-fix/lib/index.js");
/* harmony import */ var webm_duration_fix__WEBPACK_IMPORTED_MODULE_2___default = /*#__PURE__*/__webpack_require__.n(webm_duration_fix__WEBPACK_IMPORTED_MODULE_2__);
/* harmony import */ var _lib_strformat__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(/*! ../lib/strformat */ "./src/lib/strformat.js");
/* harmony import */ var _config_js__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(/*! ../config.js */ "./src/config.js");





const VIDEO_NOTE_DIM = 240;
const VIDEO_NOTE_MAX_DURATION = 60_000;
const VIDEO_MIME_TYPES = ['video/webm;codecs=vp8,opus', 'video/webm;codecs=vp9,opus', 'video/webm', 'video/mp4', ''];
const messages = (0,react_intl__WEBPACK_IMPORTED_MODULE_1__.defineMessages)({
  icon_title_delete: {
    id: "icon_title_delete_video_note",
    defaultMessage: [{
      "type": 0,
      "value": "Cancel recording"
    }]
  },
  icon_title_send: {
    id: "icon_title_send",
    defaultMessage: [{
      "type": 0,
      "value": "Send message"
    }]
  },
  failed_to_init_video: {
    id: "failed_to_init_video",
    defaultMessage: [{
      "type": 0,
      "value": "Failed to initialize video recording"
    }]
  }
});
class VideoNoteRecorder extends (react__WEBPACK_IMPORTED_MODULE_0___default().PureComponent) {
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
    this.videoRef = react__WEBPACK_IMPORTED_MODULE_0___default().createRef();
    this.durationMillis = 0;
    this.startedOn = null;
    this.recordingTimestamp = 0;
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
          width: {
            ideal: VIDEO_NOTE_DIM
          },
          height: {
            ideal: VIDEO_NOTE_DIM
          },
          facingMode: 'user',
          aspectRatio: {
            ideal: 1
          }
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
    if (this.videoRef.current) {
      this.videoRef.current.srcObject = stream;
      this.videoRef.current.play().catch(_ => {});
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
    this.mediaRecorder.ondataavailable = e => {
      if (e.data && e.data.size > 0) {
        this.videoChunks.push(e.data);
      }
    };
    this.mediaRecorder.onstop = _ => {
      const preview = this.grabPreview();
      if (!this.finished || this.durationMillis <= _config_js__WEBPACK_IMPORTED_MODULE_4__.MIN_DURATION) {
        this.props.onDeleted();
        this.cleanUp();
        return;
      }
      this.getRecording(this.mediaRecorder.mimeType).then(videoBlob => {
        this.props.onFinished(videoBlob, preview, {
          width: VIDEO_NOTE_DIM,
          height: VIDEO_NOTE_DIM,
          duration: this.durationMillis,
          mime: videoBlob.type,
          name: 'video-note.webm'
        });
      }).catch(err => this.props.onError(err.message || err)).finally(_ => this.cleanUp());
    };
    this.durationMillis = 0;
    this.startedOn = Date.now();
    this.mediaRecorder.start();
    this.setState({
      recording: true
    });
    this.tick();
    this.props.onRecordingProgress();
    this.recordingTimestamp = this.startedOn;
  }
  tick() {
    if (!this.startedOn) {
      return;
    }
    const duration = this.durationMillis + (Date.now() - this.startedOn);
    this.setState({
      duration: (0,_lib_strformat__WEBPACK_IMPORTED_MODULE_3__.secondsToTime)(duration / 1000)
    });
    const now = Date.now();
    if (now - this.recordingTimestamp > _config_js__WEBPACK_IMPORTED_MODULE_4__.KEYPRESS_DELAY) {
      this.props.onRecordingProgress();
      this.recordingTimestamp = now;
    }
    if (duration >= VIDEO_NOTE_MAX_DURATION) {
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
  grabPreview() {
    const video = this.videoRef.current;
    if (!video || !video.videoWidth || !video.videoHeight) {
      return null;
    }
    const canvas = document.createElement('canvas');
    canvas.width = VIDEO_NOTE_DIM;
    canvas.height = VIDEO_NOTE_DIM;
    const ctx = canvas.getContext('2d');
    const side = Math.min(video.videoWidth, video.videoHeight);
    const sx = (video.videoWidth - side) / 2;
    const sy = (video.videoHeight - side) / 2;
    ctx.drawImage(video, sx, sy, side, side, 0, 0, VIDEO_NOTE_DIM, VIDEO_NOTE_DIM);
    return canvas.toDataURL('image/jpeg', 0.7);
  }
  getRecording(mimeType) {
    mimeType = mimeType || VIDEO_MIME_TYPES[0];
    const blob = new Blob(this.videoChunks, {
      type: mimeType
    });
    if (mimeType.startsWith('video/webm')) {
      return webm_duration_fix__WEBPACK_IMPORTED_MODULE_2___default()(blob, mimeType).catch(_ => blob);
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
    this.setState({
      recording: false
    });
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
    const {
      formatMessage
    } = this.props.intl;
    return react__WEBPACK_IMPORTED_MODULE_0___default().createElement("div", {
      className: "video-note-recorder"
    }, react__WEBPACK_IMPORTED_MODULE_0___default().createElement("a", {
      href: "#",
      onClick: this.handleDelete,
      title: formatMessage(messages.icon_title_delete)
    }, react__WEBPACK_IMPORTED_MODULE_0___default().createElement("i", {
      className: "material-icons gray"
    }, "delete_outline")), react__WEBPACK_IMPORTED_MODULE_0___default().createElement("div", {
      className: "video-note-preview"
    }, react__WEBPACK_IMPORTED_MODULE_0___default().createElement("video", {
      ref: this.videoRef,
      muted: true,
      playsInline: true
    }), react__WEBPACK_IMPORTED_MODULE_0___default().createElement("span", {
      className: "rec-dot"
    })), react__WEBPACK_IMPORTED_MODULE_0___default().createElement("div", {
      className: "duration"
    }, this.state.duration), react__WEBPACK_IMPORTED_MODULE_0___default().createElement("a", {
      href: "#",
      onClick: this.handleDone,
      title: formatMessage(messages.icon_title_send)
    }, react__WEBPACK_IMPORTED_MODULE_0___default().createElement("i", {
      className: "material-icons"
    }, "send")));
  }
}
/* harmony default export */ __webpack_exports__["default"] = ((0,react_intl__WEBPACK_IMPORTED_MODULE_1__.injectIntl)(VideoNoteRecorder));

/***/ })

}]);
//# sourceMappingURL=src_widgets_video-note-recorder_jsx.dev.js.map