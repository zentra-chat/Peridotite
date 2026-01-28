# Zentra Peridotite API Documentation

Welcome to the Zentra backend API documentation. This document covers the RESTful API endpoints, request/response formats, and authentication.

## Table of Contents
- [General Information](#general-information)
- [Authentication](#authentication)
- [User Management](#user-management)
- [Communities](#communities)
- [Channels](#channels)
- [Messages](#messages)
- [Media & Uploads](#media--uploads)
- [WebSocket (Real-time)](#websocket-real-time)

---

## General Information

- **Base URL**: `http://localhost:8080/api/v1`
- **Response Format**: All successful responses return a JSON object with a `data` key.
- **Error Format**: Error responses return a JSON object with `error` and optional `code` and `details`.
- **Authentication**: JWT-based. Protected endpoints require the `Authorization: Bearer <token>` header.

---

## Authentication

### Register
`POST /auth/register`

**Request Body:**
```json
{
  "username": "johndoe",
  "email": "john@example.com",
  "password": "StrongPassword123!"
}
```

### Login
`POST /auth/login`

**Request Body:**
```json
{
  "login": "johndoe", // username or email
  "password": "StrongPassword123!",
  "totpCode": "123456" // optional, required if 2FA enabled
}
```

**Response Data:**
```json
{
  "user": { ... },
  "accessToken": "...",
  "refreshToken": "...",
  "expiresAt": "2026-01-27T...",
  "requires2FA": false
}
```

### Refresh Token
`POST /auth/refresh`

**Request Body:**
```json
{
  "refreshToken": "..."
}
```

### 2FA - Setup/Enable
`POST /auth/2fa/enable` (Authenticated)
Starts 2FA setup process. Returns QR code URL.

### 2FA - Verify
`POST /auth/2fa/verify` (Authenticated)
Finalizes 2FA setup by verifying a code.

---

## User Management

### Get Current User
`GET /users/me`

### Update Profile
`PATCH /users/me`

**Request Body:**
```json
{
  "displayName": "John D.",
  "bio": "Expert Coder",
  "avatarUrl": "https://..."
}
```

### Get User by ID
`GET /users/{id}`

### Search Users
`GET /users/search?q=john`

---

## Communities

### List My Communities
`GET /communities`

### Create Community
`POST /communities`

**Request Body:**
```json
{
  "name": "The Great Community",
  "description": "A place for builders",
  "isPublic": true,
  "isOpen": true
}
```

### Get Community Details
`GET /communities/{id}`

### List Members
`GET /communities/{id}/members`

### Create Invite
`POST /communities/{id}/invites`
Returns an invite code.

---

## Channels

### List Channels in Community
`GET /channels/communities/{communityId}/channels`

### Create Channel
`POST /channels/communities/{communityId}/channels`

**Request Body:**
```json
{
  "name": "general",
  "type": "text", // text, announcement, gallery, forum
  "topic": "General chat"
}
```

---

## Messages

### Get Channel Messages
`GET /messages/channels/{channelId}/messages?limit=50&before={id}`

### Send Message
`POST /messages/channels/{channelId}/messages`

**Request Body:**
```json
{
  "content": "Hello world!",
  "replyToId": "uuid", // optional
  "attachments": ["uuid", ...] // optional
}
```

### Edit Message
`PATCH /messages/{id}`

### Delete Message
`DELETE /messages/{id}`

### Add Reaction
`POST /messages/{id}/reactions`

**Request Body:**
```json
{
  "emoji": "🔥"
}
```

### Remove Reaction
`DELETE /messages/{id}/reactions/{emoji}`

---

## WebSocket (Real-time)

Connect to `/ws?token=<jwt>` to receive real-time updates.

### Supported Client Messages
- `SUBSCRIBE`: `{"type": "SUBSCRIBE", "data": {"channelId": "..."}}`
- `TYPING_START`: `{"type": "TYPING_START", "data": {"channelId": "..."}}`
- `PRESENCE_UPDATE`: `{"type": "PRESENCE_UPDATE", "data": {"status": "online"}}`

### Server Events
- `READY`
- `MESSAGE_CREATE`
- `MESSAGE_UPDATE`
- `MESSAGE_DELETE`
- `TYPING_START`
- `PRESENCE_UPDATE`
- `REACTION_ADD` (Shared via message update or dedicated event)
