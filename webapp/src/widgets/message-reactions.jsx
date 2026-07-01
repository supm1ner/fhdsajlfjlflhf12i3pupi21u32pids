// Emoji reactions shown under a chat message: a row of reaction chips plus an
// "add reaction" button that opens a small quick-picker.

import React from 'react';

// Quick reactions offered in the picker (Telegram-style set).
export const QUICK_REACTIONS = ['👍', '❤️', '😂', '😮', '😢', '🙏', '🔥', '🎉'];

export default class MessageReactions extends React.PureComponent {
  constructor(props) {
    super(props);
    this.state = {pickerOpen: false};
    this.togglePicker = this.togglePicker.bind(this);
    this.handlePick = this.handlePick.bind(this);
  }

  togglePicker(e) {
    e.preventDefault();
    e.stopPropagation();
    this.setState(prev => ({pickerOpen: !prev.pickerOpen}));
  }

  handlePick(e, emoji) {
    e.preventDefault();
    e.stopPropagation();
    this.setState({pickerOpen: false});
    this.props.onToggle(emoji);
  }

  render() {
    const reactions = this.props.reactions || [];
    if (reactions.length == 0 && !this.props.canReact) {
      return null;
    }
    return (
      <div className="reactions">
        {reactions.map(r => (
          <a href="#" key={r.emoji}
            className={'reaction' + (r.mine ? ' mine' : '')}
            onClick={e => this.handlePick(e, r.emoji)}
            title={r.mine ? 'Remove your reaction' : 'React'}>
            <span className="emoji">{r.emoji}</span>
            <span className="count">{r.count}</span>
          </a>
        ))}
        {this.props.canReact ?
          <span className="reaction-add-wrapper">
            <a href="#" className="reaction-add" onClick={this.togglePicker} title="Add reaction">
              <i className="material-icons">add_reaction</i>
            </a>
            {this.state.pickerOpen ?
              <div className="reaction-picker">
                {QUICK_REACTIONS.map(emoji => (
                  <a href="#" key={emoji} onClick={e => this.handlePick(e, emoji)}>{emoji}</a>
                ))}
              </div> : null}
          </span> : null}
      </div>
    );
  }
}
