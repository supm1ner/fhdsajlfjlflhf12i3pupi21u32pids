// A compact emoji picker panel for the message composer.
// Emits the chosen emoji via onPick; the composer inserts it at the caret.

import React from 'react';

// A curated set of popular emoji, grouped for quick scanning (Telegram-style).
const EMOJI_GROUPS = [
  {
    name: 'Smileys',
    emoji: ['😀', '😁', '😂', '🤣', '😊', '😍', '😘', '😉', '😎', '🥰', '😇', '🙂',
      '🙃', '😌', '😔', '😒', '😢', '😭', '😤', '😠', '😡', '🥺', '😳', '😱',
      '🤔', '🤨', '😐', '😴', '🤗', '🤭', '🤫', '😷', '🤒', '🤕', '🥳', '😜']
  },
  {
    name: 'Gestures',
    emoji: ['👍', '👎', '👌', '🤌', '✌️', '🤞', '🤟', '🤙', '👏', '🙌', '🙏', '💪',
      '👋', '🤝', '✍️', '👀', '🫶', '🤦', '🤷']
  },
  {
    name: 'Hearts',
    emoji: ['❤️', '🧡', '💛', '💚', '💙', '💜', '🖤', '🤍', '💔', '❣️', '💕', '💞',
      '💓', '💗', '💖', '💘', '💝', '💯']
  },
  {
    name: 'Objects',
    emoji: ['🔥', '✨', '🎉', '🎊', '🎁', '🏆', '🥇', '⭐', '🌟', '💡', '📌', '📎',
      '✅', '❌', '⚡', '💥', '🚀', '🎯', '🔔', '💤']
  },
  {
    name: 'Nature',
    emoji: ['🌸', '🌹', '🌻', '🌈', '☀️', '🌙', '⛅', '❄️', '🍀', '🐶', '🐱', '🦊',
      '🐻', '🐼', '🦁', '🐸', '🦄', '🐝', '🦋', '🌊']
  },
  {
    name: 'Food',
    emoji: ['🍏', '🍎', '🍊', '🍋', '🍉', '🍓', '🍒', '🍑', '🥭', '🍕', '🍔', '🍟',
      '🌮', '🍣', '🍩', '🍪', '🎂', '☕', '🍺', '🥂']
  }
];

export default class EmojiPicker extends React.PureComponent {
  render() {
    return (
      <div className="emoji-picker" onMouseDown={e => e.preventDefault()}>
        {EMOJI_GROUPS.map(group => (
          <div className="emoji-group" key={group.name}>
            <div className="emoji-group-title">{group.name}</div>
            <div className="emoji-grid">
              {group.emoji.map((emoji, i) => (
                <a href="#" key={group.name + i}
                  onClick={e => {e.preventDefault(); this.props.onPick(emoji);}}>{emoji}</a>
              ))}
            </div>
          </div>
        ))}
      </div>
    );
  }
}
