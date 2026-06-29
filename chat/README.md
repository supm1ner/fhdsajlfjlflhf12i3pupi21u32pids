# Sunrise Messenger Server

Instant messaging full stack. Backend in pure [Go](http://golang.org), clients for web (React), mobile, and desktop. Wire transport is JSON over websocket (long polling is also available) or protobuf with gRPC.

Sunrise is a modern, open platform for federated instant messaging with an emphasis on mobile communication. It is *not* XMPP/Jabber - it is meant as a replacement for XMPP.

## Architecture

- **Backend**: Go server with pluggable database backends (MySQL, PostgreSQL, MongoDB, RethinkDB)
- **Transport**: JSON over WebSocket / long polling, Protobuf over gRPC
- **Push notifications**: Firebase Cloud Messaging (FCM), optional Push Gateway
- **Clients**: Web (React), CLI, chatbot integrations

## Quick Start

See [INSTALL.md](INSTALL.md) for installation instructions or [docker/README.md](docker/README.md) for Docker setup.

## Project Structure

- `server/` - Go server source code
- `docker/` - Docker and Docker Compose files
- `pbx/` - Protocol Buffers definitions
- `py_grpc/` - Python gRPC client
- `tn-cli/` - Command-line client
- `monitoring/` - Prometheus/InfluxDB exporters
- `chatbot/` - Chatbot examples
- `keygen/` - Key generation utility
- `sunrise-db/` - Database tools
- `loadtest/` - Load testing scripts

## License

Server is licensed under GPL 3.0.
