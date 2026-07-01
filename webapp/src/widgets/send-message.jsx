// Send message form.
import React, { Suspense } from 'react';
import { FormattedMessage, defineMessages, injectIntl } from 'react-intl';
import { Drafty } from 'sunrise-sdk';

// Lazy-loading AudioRecorder because it's quite large due to
// a dependency on webm-duration-fix.
const AudioRecorder = React.lazy(_ => import('./audio-recorder.jsx'));
// Lazy-loading VideoNoteRecorder (кружок) for the same reason.
const VideoNoteRecorder = React.lazy(_ => import('./video-note-recorder.jsx'));
// Lazy-loading EmojiPicker: only pulled in when the user opens it.
const EmojiPicker = React.lazy(_ => import('./emoji-picker.jsx'));

import { KEYPRESS_DELAY } from '../config.js';
import { filePasted } from '../lib/blob-helpers.js';
import { replyFormatter } from '../lib/formatters.js';

const messages = defineMessages({
  messaging_disabled: {
    id: 'messaging_disabled_prompt',
    defaultMessage: 'Messaging disabled',
    description: 'Prompt in SendMessage in read-only topic'
  },
  type_new_message: {
    id: 'new_message_prompt',
    defaultMessage: 'New message',
    description: 'Prompt in send message field'
  },
  add_image_caption: {
    id: 'image_caption_prompt',
    defaultMessage: 'Image caption',
    description: 'Prompt in SendMessage for attached image'
  },
  file_attachment_too_large: {
    id: 'file_attachment_too_large',
    defaultMessage: 'The file size {size} exceeds the {limit} limit.',
    description: 'Error message when attachment is too large'
  },
  cannot_initiate_upload: {
    id: 'cannot_initiate_file_upload',
    defaultMessage: 'Cannot initiate file upload.',
    description: 'Generic error messagewhen attachment fails'
  },
  icon_title_record_voice: {
    id: 'icon_title_record_voice',
    defaultMessage: 'Record voice message',
    description: 'Icon tool tip for recording a voice message'
  },
  icon_title_emoji: {
    id: 'icon_title_emoji',
    defaultMessage: 'Emoji',
    description: 'Icon tool tip for the emoji picker'
  },
  icon_title_record_video_note: {
    id: 'icon_title_record_video_note',
    defaultMessage: 'Record video note',
    description: 'Icon tool tip for recording a round video note (кружок)'
  },
  icon_title_attach_file: {
    id: 'icon_title_attach_file',
    defaultMessage: 'Attach file',
    description: 'Icon tool tip for attaching a file'
  },
  icon_title_add_image: {
    id: 'icon_title_add_image',
    defaultMessage: 'Add image',
    description: 'Icon tool tip for attaching an image'
  },
  icon_title_send: {
    id: 'icon_title_send',
    defaultMessage: 'Send message',
    description: 'Icon tool tip for sending a message'
  },
});

class SendMessage extends React.PureComponent {
  constructor(props) {
    super(props);

    this.state = {
      quote: null,
      message: '',
      audioRec: false,
      videoNoteRec: false,
      emojiOpen: false,
      mentionMatches: [],
      mentionActive: -1,
      audioAvailable: !!(navigator.mediaDevices && navigator.mediaDevices.getUserMedia),
    };

    // Timestamp when the previous key press was sent to the server.
    this.keypressTimestamp = 0;
    // Caret index of the '@' that opened the mention dropdown (-1 = none).
    this.mentionAnchor = -1;
    // Mentions the user picked from the dropdown: [{name, uid}].
    this.pendingMentions = [];

    this.handlePasteEvent = this.handlePasteEvent.bind(this);
    this.handleAttachImage = this.handleAttachImage.bind(this);
    this.handleAttachFile = this.handleAttachFile.bind(this);
    this.handleAttachAudio = this.handleAttachAudio.bind(this);
    this.handleAttachVideoNote = this.handleAttachVideoNote.bind(this);
    this.handleInsertEmoji = this.handleInsertEmoji.bind(this);
    this.toggleEmoji = this.toggleEmoji.bind(this);
    this.selectMention = this.selectMention.bind(this);
    this.handleSend = this.handleSend.bind(this);
    this.handleKeyPress = this.handleKeyPress.bind(this);
    this.handleMessageTyping = this.handleMessageTyping.bind(this);
    this.handleDropAttach = this.handleDropAttach.bind(this);

    this.handleQuoteClick = this.handleQuoteClick.bind(this);

    this.formatReply = this.formatReply.bind(this);
  }

  componentDidMount() {
    if (this.messageEditArea) {
      this.messageEditArea.addEventListener('paste', this.handlePasteEvent, false);
      if (window.getComputedStyle(this.messageEditArea).getPropertyValue('transition-property') == 'all') {
        // Set focus on desktop, but not on mobile: focus causes soft keyboard to pop up.
        this.messageEditArea.focus();
      }
    }

    this.setState({quote: this.formatReply()});
  }

  componentWillUnmount() {
    if (this.messageEditArea) {
      this.messageEditArea.removeEventListener('paste', this.handlePasteEvent, false);
    }
  }

  componentDidUpdate(prevProps) {
    if (this.messageEditArea) {
      if (window.getComputedStyle(this.messageEditArea).getPropertyValue('transition-property') == 'all') {
        // Set focus on desktop, but not on mobile: focus causes soft keyboard to pop up.
        this.messageEditArea.focus();
      }

      // Adjust height of the message area for the amount of text.
      this.messageEditArea.style.height = '0px';
      this.messageEditArea.style.height = this.messageEditArea.scrollHeight + 'px';
    }

    if (prevProps.topicName != this.props.topicName) {
      this.mentionAnchor = -1;
      this.pendingMentions = [];
      this.setState({message: this.props.initMessage || '', audioRec: false, videoNoteRec: false,
        emojiOpen: false, mentionMatches: [], mentionActive: -1, quote: null});
    } else if (prevProps.initMessage != this.props.initMessage) {
      const msg = this.props.initMessage || '';
      this.setState({message: msg}, _ => {
        // If there is text, scroll to bottom and set caret to the end.
        this.messageEditArea.scrollTop = this.messageEditArea.scrollHeight;
        this.messageEditArea.setSelectionRange(msg.length, msg.length);
      });
    }
    if (prevProps.reply != this.props.reply) {
      this.setState({quote: this.formatReply()});
    }
  }

  formatReply() {
    return this.props.reply ?
      Drafty.format(this.props.reply.content, replyFormatter, {
        formatMessage: this.props.intl.formatMessage.bind(this.props.intl),
        authorizeURL: this.props.sunrise.authorizeURL.bind(this.props.sunrise)
      }) : null;
  }

  handlePasteEvent(e) {
    if (this.props.disabled) {
      return;
    }
    // FIXME: handle large files too.
    if (filePasted(e,
      file => { this.props.onAttachImage(file); },
      file => { this.props.onAttachFile(file); },
      this.props.onError)) {

      // If a file was pasted, don't paste base64 data into input field.
      e.preventDefault();
    }
  }

  handleAttachImage(e) {
    if (e.target.files && e.target.files.length > 0) {
      this.props.onAttachImage(e.target.files[0]);
    }
    // Clear the value so the same file can be uploaded again.
    e.target.value = '';
  }

  handleAttachFile(e) {
    if (e.target.files && e.target.files.length > 0) {
      this.props.onAttachFile(e.target.files[0]);
    }
    // Clear the value so the same file can be uploaded again.
    e.target.value = '';
  }

  handleDropAttach(files) {
    if (files && files.length > 0) {
      this.props.onAttachFile(files[0]);
    }
  }

  handleAttachAudio(url, preview, duration) {
    this.setState({audioRec: false});
    this.props.onAttachAudio(url, preview, duration);
  }

  handleAttachVideoNote(videoBlob, preview, params) {
    this.setState({videoNoteRec: false});
    this.props.onAttachVideoNote(videoBlob, preview, params);
  }

  toggleEmoji(e) {
    e.preventDefault();
    this.setState(prev => ({emojiOpen: !prev.emojiOpen}));
  }

  // Insert an emoji at the current caret position in the message input.
  handleInsertEmoji(emoji) {
    const ta = this.messageEditArea;
    const msg = this.state.message;
    let start = msg.length, end = msg.length;
    if (ta && typeof ta.selectionStart == 'number') {
      start = ta.selectionStart;
      end = ta.selectionEnd;
    }
    const next = msg.slice(0, start) + emoji + msg.slice(end);
    this.setState({message: next}, () => {
      if (this.messageEditArea) {
        const pos = start + emoji.length;
        this.messageEditArea.focus();
        this.messageEditArea.setSelectionRange(pos, pos);
      }
    });
    if (this.props.onKeyPress) {
      this.props.onKeyPress();
    }
  }

  // --- @-mention autocomplete -------------------------------------------

  // Group members eligible to be mentioned: [{user, name}] (excludes self).
  getGroupMembers() {
    const topic = (this.props.sunrise && this.props.topicName) ?
      this.props.sunrise.getTopic(this.props.topicName) : null;
    if (!topic || !topic.isGroupType || !topic.isGroupType()) {
      return [];
    }
    const members = [];
    topic.subscribers(sub => {
      if (!sub || !sub.user || sub.user == this.props.myUserId) {
        return;
      }
      const name = (sub.public && sub.public.fn) ? sub.public.fn : sub.user;
      members.push({user: sub.user, name: name});
    });
    return members;
  }

  // Detect an "@query" token ending at the caret and update the suggestion list.
  updateMentionContext(value, caret) {
    const upto = value.slice(0, caret);
    const match = /(^|\s)@([^\s@]*)$/.exec(upto);
    if (!match) {
      this.mentionAnchor = -1;
      if (this.state.mentionMatches.length) {
        this.setState({mentionMatches: [], mentionActive: -1});
      }
      return;
    }
    const query = match[2].toLowerCase();
    const members = this.getGroupMembers();
    if (members.length == 0) {
      this.mentionAnchor = -1;
      if (this.state.mentionMatches.length) {
        this.setState({mentionMatches: [], mentionActive: -1});
      }
      return;
    }
    const matches = members
      .filter(m => m.name.toLowerCase().includes(query))
      .slice(0, 8);
    // Position of the '@' character.
    this.mentionAnchor = caret - match[2].length - 1;
    this.setState({mentionMatches: matches, mentionActive: matches.length ? 0 : -1});
  }

  // Insert the picked member as "@Name " and remember it for Drafty conversion on send.
  selectMention(member) {
    const ta = this.messageEditArea;
    const caret = (ta && typeof ta.selectionStart == 'number') ? ta.selectionStart : this.state.message.length;
    const anchor = this.mentionAnchor;
    if (anchor < 0) {
      return;
    }
    const before = this.state.message.slice(0, anchor);
    const after = this.state.message.slice(caret);
    const insert = '@' + member.name + ' ';
    this.pendingMentions.push({name: member.name, uid: member.user});
    this.mentionAnchor = -1;
    this.setState({message: before + insert + after, mentionMatches: [], mentionActive: -1}, () => {
      if (this.messageEditArea) {
        const pos = (before + insert).length;
        this.messageEditArea.focus();
        this.messageEditArea.setSelectionRange(pos, pos);
      }
    });
  }

  // Convert plain text with "@Name" tokens into Drafty with clickable MN entities.
  // Returns the original string when no recorded mention is present in the text.
  buildOutgoing(text) {
    const toks = this.pendingMentions
      .filter(m => text.includes('@' + m.name))
      .map(m => ({tok: '@' + m.name, name: m.name, uid: m.uid}))
      .sort((a, b) => b.tok.length - a.tok.length);
    if (toks.length == 0) {
      return text;
    }
    let result = null;
    let plain = '';
    const flush = () => {
      if (plain) {
        const d = Drafty.parse(plain);
        result = result ? Drafty.append(result, d) : d;
        plain = '';
      }
    };
    let i = 0, used = false;
    while (i < text.length) {
      let matched = null;
      for (const t of toks) {
        if (text.startsWith(t.tok, i)) {
          const nextCh = text[i + t.tok.length];
          // Require a token boundary so "@Ann" doesn't match inside "@Anna".
          if (nextCh === undefined || /[\s.,!?;:)]/.test(nextCh)) {
            matched = t;
            break;
          }
        }
      }
      if (matched) {
        flush();
        const men = Drafty.mention(matched.name, matched.uid);
        result = result ? Drafty.append(result, men) : men;
        i += matched.tok.length;
        used = true;
      } else {
        plain += text[i];
        i++;
      }
    }
    flush();
    return used ? result : text;
  }

  handleSend(e) {
    e.preventDefault();
    const raw = this.state.message.trim();
    if (raw || this.props.acceptBlank || this.props.noInput) {
      // Convert "@Name" tokens into Drafty mentions when the user picked any.
      const outgoing = this.buildOutgoing(raw);
      this.props.onSendMessage(outgoing);
      this.pendingMentions = [];
      this.setState({message: '', emojiOpen: false, mentionMatches: [], mentionActive: -1});
    }
  }

  /* Send on Enter key */
  handleKeyPress(e) {
    if (this.state.audioRec || this.state.videoNoteRec) {
      // Ignore key presses while audio or video note is being recorded.
      e.preventDefault();
      e.stopPropagation();
      return;
    }

    // When the @-mention dropdown is open, navigate/select with the keyboard.
    const matches = this.state.mentionMatches;
    if (matches.length > 0) {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        this.setState({mentionActive: (this.state.mentionActive + 1) % matches.length});
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        this.setState({mentionActive: (this.state.mentionActive - 1 + matches.length) % matches.length});
        return;
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        const chosen = matches[this.state.mentionActive] || matches[0];
        if (chosen) {
          e.preventDefault();
          e.stopPropagation();
          this.selectMention(chosen);
          return;
        }
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        this.mentionAnchor = -1;
        this.setState({mentionMatches: [], mentionActive: -1});
        return;
      }
    }

    // Enter pressed: either send, or enter a new line.
    if (e.key === 'Enter') {
      if (this.props.sendOnEnter == 'plain') {
        // Have Shift-Enter insert a line break instead
        if (!e.shiftKey) {
          e.preventDefault();
          e.stopPropagation();

          this.handleSend(e);
        }
      } else {
        if (e.ctrlKey || e.metaKey) {
          e.preventDefault();
          e.stopPropagation();

          this.handleSend(e);
        }
      }
    }
  }

  handleMessageTyping(e) {
    this.setState({message: e.target.value});
    this.updateMentionContext(e.target.value, e.target.selectionStart);
    if (this.props.onKeyPress) {
      const now = new Date().getTime();
      if (now - this.keypressTimestamp > KEYPRESS_DELAY) {
        this.props.onKeyPress();
        this.keypressTimestamp = now;
      }
    }
  }

  handleQuoteClick(e) {
    e.preventDefault();
    e.stopPropagation();
    if (this.props.reply && this.props.onQuoteClick) {
      const replyToSeq = this.props.reply.seq;
      this.props.onQuoteClick(replyToSeq);
    }
  }

  render() {
    const { formatMessage } = this.props.intl;
    const prompt = this.props.disabled ?
      formatMessage(messages.messaging_disabled) :
      (this.props.messagePrompt ?
        formatMessage(messages[this.props.messagePrompt]) :
        formatMessage(messages.type_new_message));

    const sendIcon = (this.props.reply && this.props.reply.editing) ?
      'check_circle' : 'send';

    const quote = this.state.quote ?
      (<div id="reply-quote-preview">
        <div className="cancel">
          <a href="#" onClick={e => {e.preventDefault(); this.props.onCancelReply();}}><i className="material-icons gray">close</i></a>
        </div>
        {this.state.quote}
      </div>) : null;
    const audioEnabled = this.state.audioAvailable && this.props.onAttachAudio;
    const videoNoteEnabled = this.state.audioAvailable && this.props.onAttachVideoNote;
    const recording = this.state.audioRec || this.state.videoNoteRec;
    const emojiEnabled = !this.props.noInput && !recording;
    return (
      <div id="send-message-wrapper">
        {this.state.emojiOpen && emojiEnabled ?
          <Suspense fallback={null}>
            <EmojiPicker onPick={this.handleInsertEmoji} />
          </Suspense> : null}
        {this.state.mentionMatches.length > 0 ?
          <div className="mention-suggest" onMouseDown={e => e.preventDefault()}>
            {this.state.mentionMatches.map((m, i) => (
              <div key={m.user}
                className={'mention-item' + (i == this.state.mentionActive ? ' active' : '')}
                onClick={() => this.selectMention(m)}
                onMouseEnter={() => this.setState({mentionActive: i})}>
                <span className="mention-name">{m.name}</span>
                {m.name != m.user ? <span className="mention-id">{m.user}</span> : null}
              </div>
            ))}
          </div> : null}
        {!this.props.noInput ? quote : null}
        <div id="send-message-panel">
          {!this.props.disabled ?
            <>
              {this.props.onAttachFile && !recording ?
                <>
                  <a href="#" onClick={e => {e.preventDefault(); this.attachImage.click();}} title={formatMessage(messages.icon_title_add_image)}>
                    <i className="material-icons secondary">photo</i>
                  </a>
                  <a href="#" onClick={e => {e.preventDefault(); this.attachFile.click();}} title={formatMessage(messages.icon_title_attach_file)}>
                    <i className="material-icons secondary">attach_file</i>
                  </a>
                </>
                :
                null}
              {emojiEnabled ?
                <a href="#" className={this.state.emojiOpen ? 'active' : ''} onClick={this.toggleEmoji}
                  title={formatMessage(messages.icon_title_emoji)}>
                  <i className="material-icons secondary">mood</i>
                </a> : null}
              {this.props.noInput ?
                (quote || <div className="hr thin" />) :
                (this.state.audioRec ?
                  (<Suspense fallback={<div><FormattedMessage id="loading_note" defaultMessage="Loading..."
                  description="Message shown when component is loading"/></div>}>
                    <AudioRecorder
                      onRecordingProgress={_ => this.props.onKeyPress(true)}
                      onDeleted={_ => this.setState({audioRec: false})}
                      onError={err => {this.setState({audioRec: false}); this.props.onError(err, 'err');}}
                      onFinished={this.handleAttachAudio}/>
                  </Suspense>) :
                  this.state.videoNoteRec ?
                  (<Suspense fallback={<div><FormattedMessage id="loading_note" defaultMessage="Loading..."
                  description="Message shown when component is loading"/></div>}>
                    <VideoNoteRecorder
                      onRecordingProgress={_ => this.props.onKeyPress(true)}
                      onDeleted={_ => this.setState({videoNoteRec: false})}
                      onError={err => {this.setState({videoNoteRec: false}); this.props.onError(err, 'err');}}
                      onFinished={this.handleAttachVideoNote}/>
                  </Suspense>) :
                  <textarea id="send-message-input" placeholder={prompt}
                    value={this.state.message} onChange={this.handleMessageTyping}
                    onKeyDown={this.handleKeyPress}
                    ref={ref => {this.messageEditArea = ref;}} />)}
              {this.state.message || (!audioEnabled && !videoNoteEnabled) ?
                <a href="#" onClick={this.handleSend} title={formatMessage(messages.icon_title_send)}>
                  <i className="material-icons">{sendIcon}</i>
                </a> :
                !recording ?
                  <>
                    {videoNoteEnabled ?
                      <a href="#" onClick={e => {e.preventDefault(); this.setState({videoNoteRec: true})}} title={formatMessage(messages.icon_title_record_video_note)}>
                        <i className="material-icons">videocam</i>
                      </a> : null}
                    {audioEnabled ?
                      <a href="#" onClick={e => {e.preventDefault(); this.setState({audioRec: true})}} title={formatMessage(messages.icon_title_record_voice)}>
                        <i className="material-icons">mic</i>
                      </a> : null}
                  </> :
                  null
              }
              <input type="file" ref={ref => {this.attachFile = ref}}
                onChange={this.handleAttachFile} style={{display: 'none'}} />
              <input type="file" ref={ref => {this.attachImage = ref}} accept="image/*, video/*"
                onChange={this.handleAttachImage} style={{display: 'none'}} />
            </>
            :
            <div id="writing-disabled">{prompt}</div>
          }
        </div>
      </div>
    );
  }
};

export default injectIntl(SendMessage);
