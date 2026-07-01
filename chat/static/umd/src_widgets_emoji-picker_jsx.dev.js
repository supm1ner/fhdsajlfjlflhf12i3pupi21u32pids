"use strict";
(self["webpackChunksunrise_webapp"] = self["webpackChunksunrise_webapp"] || []).push([["src_widgets_emoji-picker_jsx"],{

/***/ "./src/widgets/emoji-picker.jsx":
/*!**************************************!*\
  !*** ./src/widgets/emoji-picker.jsx ***!
  \**************************************/
/***/ (function(__unused_webpack_module, __webpack_exports__, __webpack_require__) {

__webpack_require__.r(__webpack_exports__);
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   "default": function() { return /* binding */ EmojiPicker; }
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(/*! react */ "react");
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_0__);

const EMOJI_GROUPS = [{
  name: 'Smileys',
  emoji: ['😀', '😁', '😂', '🤣', '😊', '😍', '😘', '😉', '😎', '🥰', '😇', '🙂', '🙃', '😌', '😔', '😒', '😢', '😭', '😤', '😠', '😡', '🥺', '😳', '😱', '🤔', '🤨', '😐', '😴', '🤗', '🤭', '🤫', '😷', '🤒', '🤕', '🥳', '😜']
}, {
  name: 'Gestures',
  emoji: ['👍', '👎', '👌', '🤌', '✌️', '🤞', '🤟', '🤙', '👏', '🙌', '🙏', '💪', '👋', '🤝', '✍️', '👀', '🫶', '🤦', '🤷']
}, {
  name: 'Hearts',
  emoji: ['❤️', '🧡', '💛', '💚', '💙', '💜', '🖤', '🤍', '💔', '❣️', '💕', '💞', '💓', '💗', '💖', '💘', '💝', '💯']
}, {
  name: 'Objects',
  emoji: ['🔥', '✨', '🎉', '🎊', '🎁', '🏆', '🥇', '⭐', '🌟', '💡', '📌', '📎', '✅', '❌', '⚡', '💥', '🚀', '🎯', '🔔', '💤']
}, {
  name: 'Nature',
  emoji: ['🌸', '🌹', '🌻', '🌈', '☀️', '🌙', '⛅', '❄️', '🍀', '🐶', '🐱', '🦊', '🐻', '🐼', '🦁', '🐸', '🦄', '🐝', '🦋', '🌊']
}, {
  name: 'Food',
  emoji: ['🍏', '🍎', '🍊', '🍋', '🍉', '🍓', '🍒', '🍑', '🥭', '🍕', '🍔', '🍟', '🌮', '🍣', '🍩', '🍪', '🎂', '☕', '🍺', '🥂']
}];
class EmojiPicker extends (react__WEBPACK_IMPORTED_MODULE_0___default().PureComponent) {
  render() {
    return react__WEBPACK_IMPORTED_MODULE_0___default().createElement("div", {
      className: "emoji-picker",
      onMouseDown: e => e.preventDefault()
    }, EMOJI_GROUPS.map(group => react__WEBPACK_IMPORTED_MODULE_0___default().createElement("div", {
      className: "emoji-group",
      key: group.name
    }, react__WEBPACK_IMPORTED_MODULE_0___default().createElement("div", {
      className: "emoji-group-title"
    }, group.name), react__WEBPACK_IMPORTED_MODULE_0___default().createElement("div", {
      className: "emoji-grid"
    }, group.emoji.map((emoji, i) => react__WEBPACK_IMPORTED_MODULE_0___default().createElement("a", {
      href: "#",
      key: group.name + i,
      onClick: e => {
        e.preventDefault();
        this.props.onPick(emoji);
      }
    }, emoji))))));
  }
}

/***/ })

}]);
//# sourceMappingURL=src_widgets_emoji-picker_jsx.dev.js.map