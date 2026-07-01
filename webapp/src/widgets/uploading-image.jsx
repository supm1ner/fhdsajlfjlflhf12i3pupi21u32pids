// Image view with progress bar and a cancel button.
import React from 'react';

import FileProgress from './file-progress.jsx';

export default class UploadingImage extends React.PureComponent {
  constructor(props) {
    super(props);
  }

  render() {
    const className = 'inline-image' + (this.props['data-round'] ? ' video-note' : '');
    return (
      <div className={className}>
        {React.createElement('img', this.props)}
        <div className="rounded-container">
          <FileProgress progress={this.props.progress} onCancel={this.props.onCancelUpload} />
        </div>
      </div>
    );
  }
};
