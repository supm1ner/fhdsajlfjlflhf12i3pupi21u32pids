# Sunrise Messenger - Web App

Web client for the Sunrise instant messaging platform. Built with React.

## Features

- Real-time messaging via WebSocket
- Group chats and private messaging
- File sharing and media preview
- Audio/video calls
- Push notifications via Firebase
- PWA support (offline cache, installable)
- Multi-language support (14 languages)
- Message formatting (Drafty rich text)
- End-to-end encryption support

## Quick Start

```bash
npm install
npm run build
```

Serve the `umd/` directory with any static file server. For development:

```bash
npm run build:dev
```

## Configuration

See `src/config.js` for API keys and server address. Firebase Cloud Messaging setup is in `firebase-init.js`.

## License

Apache 2.0
