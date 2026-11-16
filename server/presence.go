package server

// Presence tracking is consolidated under PresenceStore and SocketServer.
// Native WebSocket presence (/ws/presence) and Gorilla WebSocket handlers have been
// completely eliminated in favor of JWT-authenticated Socket.IO canonical envelopes
// (presence.snapshot / presence.updated).
