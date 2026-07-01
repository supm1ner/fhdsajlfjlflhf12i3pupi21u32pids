// Built-in sticker sets. Each sticker is a glyph rendered large and chrome-less.
// The data model also supports image stickers ({ref: '<url>'}) so custom/animated
// sets can be added later without changing the picker or the renderer.

export const STICKER_SETS = [
  {
    id: 'smileys',
    title: 'Smileys',
    icon: '😀',
    stickers: ['😀', '😂', '😍', '😎', '🥳', '😭', '😡', '🤔', '😴', '🤯', '🥺', '😇',
      '🤩', '😜', '🙃', '😱', '🤗', '😬', '🤐', '🥶', '🤒', '🤠']
  },
  {
    id: 'love',
    title: 'Love',
    icon: '❤️',
    stickers: ['❤️', '🧡', '💛', '💚', '💙', '💜', '🖤', '💔', '💕', '💞', '💘', '💝',
      '😘', '🥰', '😍', '💋', '🌹', '💐']
  },
  {
    id: 'hands',
    title: 'Hands',
    icon: '👍',
    stickers: ['👍', '👎', '👌', '✌️', '🤞', '🤙', '👏', '🙌', '🙏', '💪', '👋', '🤝',
      '🤟', '🤘', '👊', '✊', '🫶', '🤌']
  },
  {
    id: 'animals',
    title: 'Animals',
    icon: '🐶',
    stickers: ['🐶', '🐱', '🦊', '🐻', '🐼', '🦁', '🐯', '🐸', '🐵', '🦄', '🐷', '🐰',
      '🐨', '🐮', '🐔', '🐧', '🦋', '🐝']
  },
  {
    id: 'celebrate',
    title: 'Party',
    icon: '🎉',
    stickers: ['🎉', '🎊', '🥳', '🎁', '🎈', '🍰', '🎂', '🍾', '🥂', '🔥', '✨', '⭐',
      '🌟', '💯', '🏆', '🥇', '🎯', '🚀']
  }
];
