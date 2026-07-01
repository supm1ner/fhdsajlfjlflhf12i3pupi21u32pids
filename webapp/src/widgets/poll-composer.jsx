// Modal dialog for creating a poll: a question and 2..10 options.

import React from 'react';
import { FormattedMessage, defineMessages, injectIntl } from 'react-intl';

const MAX_OPTIONS = 10;

const messages = defineMessages({
  question_prompt: {
    id: 'poll_question_prompt',
    defaultMessage: 'Ask a question',
    description: 'Placeholder for the poll question input'
  },
  option_prompt: {
    id: 'poll_option_prompt',
    defaultMessage: 'Option',
    description: 'Placeholder for a poll option input'
  }
});

class PollComposer extends React.PureComponent {
  constructor(props) {
    super(props);
    this.state = {question: '', options: ['', '']};
    this.handleSubmit = this.handleSubmit.bind(this);
  }

  updateOption(idx, value) {
    const options = this.state.options.slice();
    options[idx] = value;
    // Auto-append an empty option when the user fills the last one.
    if (idx == options.length - 1 && value.trim() && options.length < MAX_OPTIONS) {
      options.push('');
    }
    this.setState({options});
  }

  removeOption(idx) {
    if (this.state.options.length <= 2) {
      return;
    }
    const options = this.state.options.slice();
    options.splice(idx, 1);
    this.setState({options});
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
    const {formatMessage} = this.props.intl;
    const question = this.state.question.trim();
    const validOptions = this.state.options.map(o => o.trim()).filter(Boolean);
    const canCreate = !!question && validOptions.length >= 2;
    return (
      <div className="poll-composer-overlay" onClick={this.props.onCancel}>
        <div className="poll-composer" onClick={e => e.stopPropagation()}>
          <div className="poll-composer-title">
            <FormattedMessage id="poll_create_title" defaultMessage="New poll"
              description="Title of the poll creation dialog" />
          </div>
          <input type="text" className="poll-question-input" autoFocus
            placeholder={formatMessage(messages.question_prompt)}
            value={this.state.question}
            onChange={e => this.setState({question: e.target.value})} />
          <div className="poll-option-inputs">
            {this.state.options.map((opt, i) => (
              <div className="poll-option-row" key={i}>
                <input type="text"
                  placeholder={formatMessage(messages.option_prompt) + ' ' + (i + 1)}
                  value={opt}
                  onChange={e => this.updateOption(i, e.target.value)} />
                {this.state.options.length > 2 ?
                  <a href="#" onClick={e => {e.preventDefault(); this.removeOption(i);}}>
                    <i className="material-icons gray">close</i>
                  </a> : null}
              </div>
            ))}
          </div>
          <div className="poll-composer-actions">
            <button className="secondary" onClick={this.props.onCancel}>
              <FormattedMessage id="poll_cancel" defaultMessage="Cancel" description="Cancel poll creation" />
            </button>
            <button className="primary" disabled={!canCreate} onClick={this.handleSubmit}>
              <FormattedMessage id="poll_create" defaultMessage="Create" description="Create the poll" />
            </button>
          </div>
        </div>
      </div>
    );
  }
}

export default injectIntl(PollComposer);
