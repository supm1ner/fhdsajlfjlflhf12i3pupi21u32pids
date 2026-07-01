"use strict";
(self["webpackChunksunrise_webapp"] = self["webpackChunksunrise_webapp"] || []).push([["src_widgets_sticker-picker_jsx"],{

/***/ "./src/lib/stickers.js":
/*!*****************************!*\
  !*** ./src/lib/stickers.js ***!
  \*****************************/
/***/ (function(__unused_webpack_module, __webpack_exports__, __webpack_require__) {

__webpack_require__.r(__webpack_exports__);
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   STICKER_SETS: function() { return /* binding */ STICKER_SETS; }
/* harmony export */ });
const STICKER_SETS = [{
  id: 'smileys',
  title: 'Smileys',
  icon: '😀',
  stickers: ['😀', '😂', '😍', '😎', '🥳', '😭', '😡', '🤔', '😴', '🤯', '🥺', '😇', '🤩', '😜', '🙃', '😱', '🤗', '😬', '🤐', '🥶', '🤒', '🤠']
}, {
  id: 'love',
  title: 'Love',
  icon: '❤️',
  stickers: ['❤️', '🧡', '💛', '💚', '💙', '💜', '🖤', '💔', '💕', '💞', '💘', '💝', '😘', '🥰', '😍', '💋', '🌹', '💐']
}, {
  id: 'hands',
  title: 'Hands',
  icon: '👍',
  stickers: ['👍', '👎', '👌', '✌️', '🤞', '🤙', '👏', '🙌', '🙏', '💪', '👋', '🤝', '🤟', '🤘', '👊', '✊', '🫶', '🤌']
}, {
  id: 'animals',
  title: 'Animals',
  icon: '🐶',
  stickers: ['🐶', '🐱', '🦊', '🐻', '🐼', '🦁', '🐯', '🐸', '🐵', '🦄', '🐷', '🐰', '🐨', '🐮', '🐔', '🐧', '🦋', '🐝']
}, {
  id: 'celebrate',
  title: 'Party',
  icon: '🎉',
  stickers: ['🎉', '🎊', '🥳', '🎁', '🎈', '🍰', '🎂', '🍾', '🥂', '🔥', '✨', '⭐', '🌟', '💯', '🏆', '🥇', '🎯', '🚀']
}];

/***/ }),

/***/ "./src/widgets/sticker-picker.jsx":
/*!****************************************!*\
  !*** ./src/widgets/sticker-picker.jsx ***!
  \****************************************/
/***/ (function(__unused_webpack_module, __webpack_exports__, __webpack_require__) {

__webpack_require__.r(__webpack_exports__);
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   "default": function() { return /* binding */ StickerPicker; }
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(/*! react */ "react");
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var _lib_stickers_js__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(/*! ../lib/stickers.js */ "./src/lib/stickers.js");


class StickerPicker extends (react__WEBPACK_IMPORTED_MODULE_0___default().PureComponent) {
  constructor(props) {
    super(props);
    this.state = {
      activeSet: _lib_stickers_js__WEBPACK_IMPORTED_MODULE_1__.STICKER_SETS[0].id
    };
  }
  renderSticker(sticker, key) {
    if (typeof sticker == 'object' && sticker.ref) {
      return react__WEBPACK_IMPORTED_MODULE_0___default().createElement("img", {
        src: this.props.authorizeURL ? this.props.authorizeURL(sticker.ref) : sticker.ref,
        alt: ""
      });
    }
    return react__WEBPACK_IMPORTED_MODULE_0___default().createElement("span", {
      className: "sticker-glyph"
    }, sticker);
  }
  render() {
    const set = _lib_stickers_js__WEBPACK_IMPORTED_MODULE_1__.STICKER_SETS.find(s => s.id == this.state.activeSet) || _lib_stickers_js__WEBPACK_IMPORTED_MODULE_1__.STICKER_SETS[0];
    return react__WEBPACK_IMPORTED_MODULE_0___default().createElement("div", {
      className: "sticker-picker",
      onMouseDown: e => e.preventDefault()
    }, react__WEBPACK_IMPORTED_MODULE_0___default().createElement("div", {
      className: "sticker-grid"
    }, set.stickers.map((sticker, i) => react__WEBPACK_IMPORTED_MODULE_0___default().createElement("a", {
      href: "#",
      key: i,
      className: "sticker-item",
      onClick: e => {
        e.preventDefault();
        this.props.onPick(sticker);
      }
    }, this.renderSticker(sticker, i)))), react__WEBPACK_IMPORTED_MODULE_0___default().createElement("div", {
      className: "sticker-tabs"
    }, _lib_stickers_js__WEBPACK_IMPORTED_MODULE_1__.STICKER_SETS.map(s => react__WEBPACK_IMPORTED_MODULE_0___default().createElement("a", {
      href: "#",
      key: s.id,
      className: 'sticker-tab' + (s.id == this.state.activeSet ? ' active' : ''),
      onClick: e => {
        e.preventDefault();
        this.setState({
          activeSet: s.id
        });
      },
      title: s.title
    }, s.icon))));
  }
}

/***/ })

}]);
//# sourceMappingURL=src_widgets_sticker-picker_jsx.dev.js.map