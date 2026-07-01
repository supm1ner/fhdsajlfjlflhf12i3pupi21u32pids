// Full-screen story viewer: segmented progress bars, auto-advance, tap to
// navigate. Closes when the last story finishes or the user dismisses it.

import React from 'react';

const STORY_DURATION_MS = 5000;
const TICK_MS = 50;

export default class StoryViewer extends React.PureComponent {
  constructor(props) {
    super(props);
    this.state = {index: 0, progress: 0};
    this.tick = this.tick.bind(this);
    this.next = this.next.bind(this);
    this.prev = this.prev.bind(this);
    this.handleKey = this.handleKey.bind(this);
  }

  componentDidMount() {
    this.start();
    document.addEventListener('keydown', this.handleKey);
  }

  componentWillUnmount() {
    clearInterval(this.timer);
    document.removeEventListener('keydown', this.handleKey);
  }

  start() {
    clearInterval(this.timer);
    this.setState({progress: 0});
    this.elapsed = 0;
    this.timer = setInterval(this.tick, TICK_MS);
  }

  tick() {
    this.elapsed += TICK_MS;
    const progress = Math.min(1, this.elapsed / STORY_DURATION_MS);
    this.setState({progress});
    if (progress >= 1) {
      this.next();
    }
  }

  next() {
    const {index} = this.state;
    if (index + 1 >= this.props.stories.length) {
      clearInterval(this.timer);
      this.props.onClose();
      return;
    }
    this.setState({index: index + 1}, () => this.start());
  }

  prev() {
    const index = Math.max(0, this.state.index - 1);
    this.setState({index}, () => this.start());
  }

  handleKey(e) {
    if (e.key === 'Escape') {
      this.props.onClose();
    } else if (e.key === 'ArrowRight') {
      this.next();
    } else if (e.key === 'ArrowLeft') {
      this.prev();
    }
  }

  render() {
    const {stories} = this.props;
    const {index, progress} = this.state;
    const story = stories[index];
    if (!story) {
      return null;
    }
    return (
      <div className="story-viewer-overlay" onClick={this.props.onClose}>
        <div className="story-viewer" onClick={e => e.stopPropagation()}>
          <div className="story-progress">
            {stories.map((s, i) => (
              <div className="story-progress-track" key={s.id}>
                <div className="story-progress-fill"
                  style={{width: (i < index ? 100 : i == index ? progress * 100 : 0) + '%'}} />
              </div>
            ))}
          </div>
          <a href="#" className="story-close" onClick={e => {e.preventDefault(); this.props.onClose();}}>
            <i className="material-icons">close</i>
          </a>
          <img className="story-image" src={story.img} alt="" />
          <div className="story-nav">
            <a href="#" className="story-nav-prev" onClick={e => {e.preventDefault(); this.prev();}} />
            <a href="#" className="story-nav-next" onClick={e => {e.preventDefault(); this.next();}} />
          </div>
        </div>
      </div>
    );
  }
}
