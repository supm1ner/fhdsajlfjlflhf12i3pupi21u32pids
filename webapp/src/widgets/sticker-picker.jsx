// Sticker picker: tabbed sticker sets. Picking a sticker sends it immediately as
// a standalone (chrome-less, oversized) message.

import React from 'react';

import { STICKER_SETS } from '../lib/stickers.js';

export default class StickerPicker extends React.PureComponent {
  constructor(props) {
    super(props);
    this.state = {activeSet: STICKER_SETS[0].id};
  }

  renderSticker(sticker, key) {
    // Image stickers ({ref}) render as <img>; glyph stickers render as text.
    if (typeof sticker == 'object' && sticker.ref) {
      return <img src={this.props.authorizeURL ? this.props.authorizeURL(sticker.ref) : sticker.ref} alt="" />;
    }
    return <span className="sticker-glyph">{sticker}</span>;
  }

  render() {
    const set = STICKER_SETS.find(s => s.id == this.state.activeSet) || STICKER_SETS[0];
    return (
      <div className="sticker-picker" onMouseDown={e => e.preventDefault()}>
        <div className="sticker-grid">
          {set.stickers.map((sticker, i) => (
            <a href="#" key={i} className="sticker-item"
              onClick={e => {e.preventDefault(); this.props.onPick(sticker);}}>
              {this.renderSticker(sticker, i)}
            </a>
          ))}
        </div>
        <div className="sticker-tabs">
          {STICKER_SETS.map(s => (
            <a href="#" key={s.id}
              className={'sticker-tab' + (s.id == this.state.activeSet ? ' active' : '')}
              onClick={e => {e.preventDefault(); this.setState({activeSet: s.id});}}
              title={s.title}>{s.icon}</a>
          ))}
        </div>
      </div>
    );
  }
}
