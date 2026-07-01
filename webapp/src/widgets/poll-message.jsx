// Renders a poll inside a chat message: a question and clickable options with
// live vote bars. Voting is handled by the parent (aggregated like reactions).

import React from 'react';
import { FormattedMessage } from 'react-intl';

export default class PollMessage extends React.PureComponent {
  render() {
    const {question, options, counts, total, myVote, canVote, onVote} = this.props;
    return (
      <div className="poll">
        <div className="poll-question">{question}</div>
        <div className="poll-options">
          {options.map((opt, i) => {
            const c = counts[i] || 0;
            const pct = total > 0 ? Math.round((c * 100) / total) : 0;
            const mine = myVote === i;
            return (
              <a href="#" key={i}
                className={'poll-option' + (mine ? ' mine' : '') + (canVote ? '' : ' disabled')}
                onClick={e => {e.preventDefault(); if (canVote) { onVote(i); }}}>
                <span className="poll-bar" style={{width: pct + '%'}} />
                <span className="poll-opt-body">
                  <span className="poll-opt-text">{mine ? '✓ ' : ''}{opt}</span>
                  <span className="poll-opt-count">{pct}%</span>
                </span>
              </a>
            );
          })}
        </div>
        <div className="poll-total">
          <FormattedMessage id="poll_votes" defaultMessage="{count, plural, one {# vote} other {# votes}}"
            description="Total number of votes in a poll" values={{count: total}} />
        </div>
      </div>
    );
  }
}
