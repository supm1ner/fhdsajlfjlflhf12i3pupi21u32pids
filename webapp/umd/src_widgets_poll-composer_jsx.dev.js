"use strict";
(self["webpackChunksunrise_webapp"] = self["webpackChunksunrise_webapp"] || []).push([["src_widgets_poll-composer_jsx"],{

/***/ "./src/widgets/poll-composer.jsx":
/*!***************************************!*\
  !*** ./src/widgets/poll-composer.jsx ***!
  \***************************************/
/***/ (function(__unused_webpack_module, __webpack_exports__, __webpack_require__) {

__webpack_require__.r(__webpack_exports__);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(/*! react */ "react");
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var react_intl__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(/*! react-intl */ "react-intl");
/* harmony import */ var react_intl__WEBPACK_IMPORTED_MODULE_1___default = /*#__PURE__*/__webpack_require__.n(react_intl__WEBPACK_IMPORTED_MODULE_1__);


const MAX_OPTIONS = 10;
const messages = (0,react_intl__WEBPACK_IMPORTED_MODULE_1__.defineMessages)({
  question_prompt: {
    id: "poll_question_prompt",
    defaultMessage: [{
      "type": 0,
      "value": "Ask a question"
    }]
  },
  option_prompt: {
    id: "poll_option_prompt",
    defaultMessage: [{
      "type": 0,
      "value": "Option"
    }]
  }
});
class PollComposer extends (react__WEBPACK_IMPORTED_MODULE_0___default().PureComponent) {
  constructor(props) {
    super(props);
    this.state = {
      question: '',
      options: ['', '']
    };
    this.handleSubmit = this.handleSubmit.bind(this);
  }
  updateOption(idx, value) {
    const options = this.state.options.slice();
    options[idx] = value;
    if (idx == options.length - 1 && value.trim() && options.length < MAX_OPTIONS) {
      options.push('');
    }
    this.setState({
      options
    });
  }
  removeOption(idx) {
    if (this.state.options.length <= 2) {
      return;
    }
    const options = this.state.options.slice();
    options.splice(idx, 1);
    this.setState({
      options
    });
  }
  handleSubmit(e) {
    e.preventDefault();
    const question = this.state.question.trim();
    const options = this.state.options.map(o => o.trim()).filter(Boolean);
    if (question && options.length >= 2) {
      this.props.onCreate(question, options);
    }
  }
  render() {
    const {
      formatMessage
    } = this.props.intl;
    const question = this.state.question.trim();
    const validOptions = this.state.options.map(o => o.trim()).filter(Boolean);
    const canCreate = !!question && validOptions.length >= 2;
    return react__WEBPACK_IMPORTED_MODULE_0___default().createElement("div", {
      className: "poll-composer-overlay",
      onClick: this.props.onCancel
    }, react__WEBPACK_IMPORTED_MODULE_0___default().createElement("div", {
      className: "poll-composer",
      onClick: e => e.stopPropagation()
    }, react__WEBPACK_IMPORTED_MODULE_0___default().createElement("div", {
      className: "poll-composer-title"
    }, react__WEBPACK_IMPORTED_MODULE_0___default().createElement(react_intl__WEBPACK_IMPORTED_MODULE_1__.FormattedMessage, {
      id: "poll_create_title",
      defaultMessage: [{
        "type": 0,
        "value": "New poll"
      }]
    })), react__WEBPACK_IMPORTED_MODULE_0___default().createElement("input", {
      type: "text",
      className: "poll-question-input",
      autoFocus: true,
      placeholder: formatMessage(messages.question_prompt),
      value: this.state.question,
      onChange: e => this.setState({
        question: e.target.value
      })
    }), react__WEBPACK_IMPORTED_MODULE_0___default().createElement("div", {
      className: "poll-option-inputs"
    }, this.state.options.map((opt, i) => react__WEBPACK_IMPORTED_MODULE_0___default().createElement("div", {
      className: "poll-option-row",
      key: i
    }, react__WEBPACK_IMPORTED_MODULE_0___default().createElement("input", {
      type: "text",
      placeholder: formatMessage(messages.option_prompt) + ' ' + (i + 1),
      value: opt,
      onChange: e => this.updateOption(i, e.target.value)
    }), this.state.options.length > 2 ? react__WEBPACK_IMPORTED_MODULE_0___default().createElement("a", {
      href: "#",
      onClick: e => {
        e.preventDefault();
        this.removeOption(i);
      }
    }, react__WEBPACK_IMPORTED_MODULE_0___default().createElement("i", {
      className: "material-icons gray"
    }, "close")) : null))), react__WEBPACK_IMPORTED_MODULE_0___default().createElement("div", {
      className: "poll-composer-actions"
    }, react__WEBPACK_IMPORTED_MODULE_0___default().createElement("button", {
      className: "secondary",
      onClick: this.props.onCancel
    }, react__WEBPACK_IMPORTED_MODULE_0___default().createElement(react_intl__WEBPACK_IMPORTED_MODULE_1__.FormattedMessage, {
      id: "poll_cancel",
      defaultMessage: [{
        "type": 0,
        "value": "Cancel"
      }]
    })), react__WEBPACK_IMPORTED_MODULE_0___default().createElement("button", {
      className: "primary",
      disabled: !canCreate,
      onClick: this.handleSubmit
    }, react__WEBPACK_IMPORTED_MODULE_0___default().createElement(react_intl__WEBPACK_IMPORTED_MODULE_1__.FormattedMessage, {
      id: "poll_create",
      defaultMessage: [{
        "type": 0,
        "value": "Create"
      }]
    })))));
  }
}
/* harmony default export */ __webpack_exports__["default"] = ((0,react_intl__WEBPACK_IMPORTED_MODULE_1__.injectIntl)(PollComposer));

/***/ })

}]);
//# sourceMappingURL=src_widgets_poll-composer_jsx.dev.js.map