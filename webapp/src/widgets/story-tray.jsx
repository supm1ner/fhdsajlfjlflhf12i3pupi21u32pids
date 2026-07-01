// Story tray shown above the chat list: the user's own story ring with an add
// button, plus a full-screen viewer. Device-local for now (see stories.js /
// docs/STORIES_AND_BOTS.md for cross-user distribution).

import React from 'react';
import { FormattedMessage } from 'react-intl';

import StoryViewer from './story-viewer.jsx';
import { loadStories, addStory } from '../lib/stories.js';
import { imageScaled, blobToBase64 } from '../lib/blob-helpers.js';
import { MAX_EXTERN_ATTACHMENT_SIZE } from '../config.js';

// Stories are downscaled before being stored to keep localStorage small.
const STORY_DIM = 720;

export default class StoryTray extends React.PureComponent {
  constructor(props) {
    super(props);
    this.state = {stories: loadStories(props.myUserId), viewerOpen: false};
    this.handleAdd = this.handleAdd.bind(this);
  }

  handleAdd(e) {
    const file = e.target.files && e.target.files[0];
    e.target.value = '';
    if (!file) {
      return;
    }
    imageScaled(file, STORY_DIM, STORY_DIM, MAX_EXTERN_ATTACHMENT_SIZE, false)
      .then(scaled => blobToBase64(scaled.blob))
      .then(b64 => {
        const dataUrl = 'data:' + b64.mime + ';base64,' + b64.bits;
        const stories = addStory(this.props.myUserId, dataUrl);
        this.setState({stories});
      })
      .catch(err => this.props.onError && this.props.onError(err.message, 'err'));
  }

  render() {
    const has = this.state.stories.length > 0;
    return (
      <div className="story-tray">
        <div className="story-cell">
          <div className={'story-ring' + (has ? ' has' : '')}
            onClick={_ => { if (has) { this.setState({viewerOpen: true}); } else { this.fileInput.click(); } }}>
            {has ?
              <img src={this.state.stories[this.state.stories.length - 1].img} alt="" /> :
              <i className="material-icons">add</i>}
          </div>
          {has ?
            <a href="#" className="story-add-badge" onClick={e => {e.preventDefault(); this.fileInput.click();}}>
              <i className="material-icons">add</i>
            </a> : null}
          <span className="story-label">
            <FormattedMessage id="your_story" defaultMessage="Your story"
              description="Label under the user's own story in the story tray" />
          </span>
        </div>
        <input type="file" accept="image/*" ref={ref => {this.fileInput = ref;}}
          onChange={this.handleAdd} style={{display: 'none'}} />
        {this.state.viewerOpen ?
          <StoryViewer stories={this.state.stories} onClose={_ => this.setState({viewerOpen: false})} /> : null}
      </div>
    );
  }
}
