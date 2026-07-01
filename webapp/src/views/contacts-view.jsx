/* ContactsView holds all contacts-related stuff */
import React from 'react';
import { FormattedMessage, defineMessages, injectIntl } from 'react-intl';
import { Sunrise } from 'sunrise-sdk';

import ContactList from '../widgets/contact-list.jsx';
import StoryTray from '../widgets/story-tray.jsx';

import { updateFavicon } from '../lib/utils.js';

const messages = defineMessages({
  archived_contacts_title: {
    id: "archived_contacts",
    defaultMessage: "Archived contacts ({count})",
    description: "Label for archived chats"
  },
  folder_all: {
    id: 'folder_all',
    defaultMessage: 'All',
    description: 'Chat-list folder tab: all chats'
  },
  folder_unread: {
    id: 'folder_unread',
    defaultMessage: 'Unread',
    description: 'Chat-list folder tab: unread chats'
  },
  folder_direct: {
    id: 'folder_direct',
    defaultMessage: 'Direct',
    description: 'Chat-list folder tab: one-on-one chats'
  },
  folder_groups: {
    id: 'folder_groups',
    defaultMessage: 'Groups',
    description: 'Chat-list folder tab: group chats and channels'
  }
});

// Chat-list folders (Telegram-style filter tabs).
const FOLDERS = [
  {id: 'all', label: messages.folder_all},
  {id: 'unread', label: messages.folder_unread},
  {id: 'direct', label: messages.folder_direct},
  {id: 'groups', label: messages.folder_groups}
];

function isGroupLike(topic) {
  return !!topic && (Sunrise.isGroupTopicName(topic) || Sunrise.isChannelTopicName(topic));
}

class ContactsView extends React.Component {
  constructor(props) {
    super(props);

    this.handleAction = this.handleAction.bind(this);

    this.state = ContactsView.deriveStateFromProps(props);
    // Active chat-list folder (filter tab).
    this.state.folder = 'all';
  }

  static deriveStateFromProps(props) {
    const contacts = [];
    let unreadThreads = 0;
    let archivedCount = 0;

    const me = props.sunrise.getMeTopic();
    props.chatList.forEach(c => {
      const blocked = c.acs && !c.acs.isJoiner();
      c.pinned = me.pinnedTopicRank(c.topic);

      // Show only blocked contacts only when props.blocked == true.
      if (blocked && props.blocked) {
        contacts.push(c);
      }
      if (blocked || props.blocked) {
        return;
      }

      if (c.private && c.private.arch) {
        if (props.archive) {
          contacts.push(c);
        } else {
          archivedCount ++;
        }
      } else if (!props.archive) {
        contacts.push(c);
        unreadThreads += c.unread > 0 ? 1 : 0;
      }
    });

    contacts.sort((a, b) => {
      const pin = b.pinned - a.pinned;
      return pin != 0 ? pin : (b.touched || 0) - (a.touched || 0);
    });

    if (archivedCount > 0) {
      contacts.push({
        action: 'archive',
        title: messages.archived_contacts_title,
        values: {count: archivedCount}
      });
    }

    return {
      contactList: contacts,
      unreadThreads: unreadThreads
    };
  }

  componentDidUpdate(prevProps, prevState) {
    if (prevProps.chatList != this.props.chatList ||
        prevProps.archive != this.props.archive ||
        prevProps.blocked != this.props.blocked) {
      const newState = ContactsView.deriveStateFromProps(this.props);
      this.setState(newState);
      if (newState.unreadThreads != prevState.unreadThreads) {
        updateFavicon(newState.unreadThreads);
      }
    }
  }

  handleAction(action_ignored) {
    this.props.onShowArchive();
  }

  // Applies the active folder filter, always keeping special action rows (e.g. archive).
  filteredContacts() {
    const folder = this.state.folder;
    if (folder == 'all') {
      return this.state.contactList;
    }
    return this.state.contactList.filter(c => {
      if (c.action) {
        return true;
      }
      switch (folder) {
        case 'unread': return c.unread > 0;
        case 'direct': return !isGroupLike(c.topic);
        case 'groups': return isGroupLike(c.topic);
        default: return true;
      }
    });
  }

  render() {
    const { formatMessage } = this.props.intl;
    // Folders only make sense for the main chat list, not archived/blocked views.
    const showFolders = !this.props.archive && !this.props.blocked && this.state.contactList.length > 0;
    const folderTabs = showFolders ? (
      <div id="chat-folders">
        {FOLDERS.map(f => (
          <a key={f.id} href="#"
            className={'folder-tab' + (this.state.folder == f.id ? ' active' : '')}
            onClick={e => {e.preventDefault(); this.setState({folder: f.id});}}>
            {formatMessage(f.label)}
            {f.id == 'unread' && this.state.unreadThreads > 0 ?
              <span className="folder-badge">{this.state.unreadThreads}</span> : null}
          </a>
        ))}
      </div>
    ) : null;

    return (
      <>
        {showFolders && this.props.myUserId ?
          <StoryTray myUserId={this.props.myUserId} onError={this.props.onError} /> : null}
        {folderTabs}
        <FormattedMessage id="contacts_not_found"
          defaultMessage="You have no chats\n¯∖_(ツ)_/¯"
          description="HTML message shown in ContactList when no contacts are found">{
          (no_contacts) => <ContactList
            sunrise={this.props.sunrise}
            connected={this.props.connected}
            contacts={this.filteredContacts()}
            emptyListMessage={no_contacts}
            topicSelected={this.props.topicSelected}
            myUserId={this.props.myUserId}
            showOnline={true}
            showUnread={true}
            onTopicSelected={this.props.onTopicSelected}
            showContextMenu={this.props.showContextMenu}
            onAction={this.handleAction} />
        }</FormattedMessage>
      </>
    );
  }
};

export default injectIntl(ContactsView);
