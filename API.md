# Zentra Peridotite API Documentation

Welcome to the Zentra backend API documentation. This document covers RESTful endpoints, request/response JSON structures, and real-time WebSocket communication.

## Table of Contents
- [General Information](#general-information)
- [Common Data Models](#common-data-models)
- [Authentication](#authentication)
- [User Management](#user-management)
- [Communities](#communities)
- [Channels & Categories](#channels--categories)
- [Messages & Reactions](#messages--reactions)
- [Media & Uploads](#media--uploads)
- [WebSocket (Real-time)](#websocket-real-time)

---

## General Information

- **Base URL**: `http://localhost:8080/api/v1`
- **Content Type**: `application/json` for all requests and responses.
- **Authentication**: JWT-based via `Authorization: Bearer <token>` header.
- **Success Responses**: All successful non-paginated responses are wrapped in a JSON object with a `data` key.
  ```json
  {
    "data": { ... } // or [ ... ]
  }
  ```
- **Paginated Responses**: Used for lists that support paging.
  ```json
  {
    "data": [ ... ],
    "total": 100,
    "page": 1,
    "pageSize": 20,
    "totalPages": 5
  }
  ```
- **Error Responses**:
  ```json
  {
    "error": "Short description of the error",
    "code": "ERROR_CODE",
    "details": { ... } // Optional object for validation errors
  }
  ```

---

## Common Data Models

These models represent the structure of the JSON objects returned in the `data` field.

### User (Public)
```json
{
  "id": "uuid",
  "username": "string",
  "displayName": "string | null",
  "avatarUrl": "string | null",
  "bio": "string | null",
  "status": "online | away | busy | invisible | offline",
  "customStatus": "string | null"
}
```

### Community
```json
{
  "id": "uuid",
  "name": "string",
  "description": "string | null",
  "iconUrl": "string | null",
  "bannerUrl": "string | null",
  "ownerId": "uuid",
  "isPublic": "boolean",
  "isOpen": "boolean",
  "memberCount": "number",
  "createdAt": "iso-date",
  "updatedAt": "iso-date"
}
```

### Channel
```json
{
  "id": "uuid",
  "communityId": "uuid",
  "categoryId": "uuid | null",
  "name": "string",
  "topic": "string | null",
  "type": "text | announcement | gallery | forum",
  "position": "number",
  "isNsfw": "boolean",
  "slowmodeSeconds": "number",
  "createdAt": "iso-date",
  "updatedAt": "iso-date"
}
```

### Message
```json
{
  "id": "uuid",
  "channelId": "uuid",
  "authorId": "uuid",
  "content": "string | null",
  "replyToId": "uuid | null",
  "isEdited": "boolean",
  "isPinned": "boolean",
  "reactions": [
    {
      "emoji": "string",
      "count": "number",
      "reacted": "boolean"
    }
  ],
  "author": { ...User (Public)... },
  "attachments": [ ...Attachment... ],
  "createdAt": "iso-date",
  "updatedAt": "iso-date"
}
```

---

## Authentication

### Register
- **Path**: `POST /auth/register`
- **Request**:
  ```json
  {
    "username": "string",
    "email": "string",
    "password": "string"
  }
  ```
- **Response**: `AuthResponse`
  ```json
  {
    "data": {
      "user": { ...Full User... },
      "accessToken": "string",
      "refreshToken": "string",
      "expiresAt": "iso-date"
    }
  }
  ```

### Login
- **Path**: `POST /auth/login`
- **Request**:
  ```json
  {
    "login": "username_or_email",
    "password": "string",
    "totpCode": "string" // Optional
  }
  ```
- **Response**: `AuthResponse`
  ```json
  {
    "data": {
      "user": { ...Full User... },
      "accessToken": "string",
      "refreshToken": "string",
      "expiresAt": "iso-date",
      "requires2FA": "boolean"
    }
  }
  ```

### Refresh Token
- **Path**: `POST /auth/refresh`
- **Request**: `{"refreshToken": "string"}`
- **Response**: `AuthResponse`

---

## User Management

### Get Current User
- **Path**: `GET /users/me` (Auth required)
- **Response**: `{"data": { ...Full User... }}`

### Update Profile
- **Path**: `PATCH /users/me` (Auth required)
- **Request**:
  ```json
  {
    "displayName": "string | null",
    "bio": "string | null",
    "customStatus": "string | null"
  }
  ```
- **Response**: `{"data": { ...Full User... }}`

### Update Status
- **Path**: `PUT /users/me/status` (Auth required)
- **Request**: `{"status": "online"}`
- **Response**: `204 No Content`

### Get User Settings
- **Path**: `GET /users/me/settings` (Auth required)
- **Response**: `{"data": { ...UserSettings... }}`

### Update User Settings
- **Path**: `PATCH /users/me/settings` (Auth required)
- **Request**:
  ```json
  {
    "theme": "string",
    "notificationsEnabled": "boolean",
    "soundEnabled": "boolean",
    "compactMode": "boolean",
    "settings": "object"
  }
  ```
- **Response**: `{"data": { ...UserSettings... }}`

### Search Users
- **Path**: `GET /users/search?q={query}&page=1&pageSize=20` (Auth required)
- **Response**: `PaginatedResponse<User (Public)>`

---

## Communities

### Discover
- **Path**: `GET /communities/discover?q={query}&page=1&pageSize=20`
- **Response**: `PaginatedResponse<Community>`

### Create
- **Path**: `POST /communities` (Auth required)
- **Request**:
  ```json
  {
    "name": "string",
    "description": "string | null",
    "isPublic": "boolean",
    "isOpen": "boolean"
  }
  ```
- **Response**: `{"data": { ...Community... }}`

### Member Management
- **List Members**: `GET /communities/{id}/members?page=1&pageSize=50` (Auth required)
  - **Response**: `PaginatedResponse<CommunityMemberWithUser>`
- **Join**: `POST /communities/{id}/join` (Auth required)
  - **Response**: `204 No Content`
- **Leave**: `POST /communities/{id}/leave` (Auth required)
  - **Response**: `204 No Content`

### Invites
- **Get Invite Info**: `GET /communities/invite/{code}` (Public)
  - **Response**: `{"data": { "community": { ...Community... }, "valid": true }}`
- **Join with Invite**: `POST /communities/join/{code}` (Auth required)
  - **Response**: `{"data": { ...Community... }}`

---

## Channels & Categories

### Categories
- **List Categories**: `GET /communities/{communityId}/categories` (Auth required)
  - **Response**: `{"data": [ ...ChannelCategory... ]}`
- **Create Category**: `POST /communities/{communityId}/categories` (Auth required)
  - **Request**: `{"name": "string"}`
  - **Response**: `{"data": { ...ChannelCategory... }}`

### Channels
- **List Channels**: `GET /communities/{communityId}/channels` (Auth required)
  - **Response**: `{"data": [ ...Channel... ]}`
- **Create Channel**: `POST /communities/{communityId}/channels` (Auth required)
  - **Request**:
    ```json
    {
      "name": "string",
      "type": "text | announcement | gallery | forum",
      "topic": "string | null",
      "categoryId": "uuid | null",
      "isNsfw": "boolean"
    }
    ```
  - **Response**: `{"data": { ...Channel... }}`

### Channel Permissions
- `GET /channels/{id}/permissions` - Get permission overwrites
- `PUT /channels/{id}/permissions` - Set permission overwrite
- `DELETE /channels/{id}/permissions/{targetType}/{targetId}` - Remove overwrite

---

## Messages & Reactions

### Fetching Messages
- `GET /messages/channels/{channelId}/messages?limit=50&before={id}&after={id}`
- `GET /messages/channels/{channelId}/messages/pinned`
- `GET /messages/channels/{channelId}/messages/search?q={query}`

### Sending & Editing
- `POST /messages/channels/{channelId}/messages` - Send message
- `PATCH /messages/{id}` - Edit message content
- `DELETE /messages/{id}` - Delete message
- `POST /messages/channels/{channelId}/messages/typing` - Send typing indicator

**Send Message Request Body:**
```json
{
  "content": "Hello world!",
  "replyToId": "uuid", // optional
  "attachments": ["uuid", ...] // optional attachment IDs
}
```

### Reactions & Pinning
- `POST /messages/{id}/reactions` - Add reaction (`{"emoji": "�"}`)
- `DELETE /messages/{id}/reactions/{emoji}` - Remove reaction
- `POST /messages/{id}/pin` - Pin message
- `DELETE /messages/{id}/pin` - Unpin message

---

## Media & Uploads

Files are uploaded first to obtain an ID, which can then be attached to messages or used for profile assets.

### Attachments
- `POST /media/attachments` - Upload a file (Multipart form, field: `file`)
- `GET /media/attachments/{id}` - Get attachment metadata
- `GET /media/attachments/{id}/download` - Get a temporary download URL

### Avatars & Assets
- `POST /media/avatars/user` - Upload user avatar (field: `avatar`)
- `POST /media/avatars/community/{communityId}` - Upload community icon (field: `avatar`)
- `POST /media/communities/{communityId}/banner` - Upload community banner (field: `banner`)

### Limits & Supported Types
- **Max File Sizes**:
  - Avatars: 5MB
  - Community Banners: 10MB
  - Generic Attachments: 50MB
  - Videos: 100MB
- **Supported Formats**:
  - Images: JPEG, PNG, GIF, WebP
  - Videos: MP4, WebM, QuickTime
  - Audio: MP3, OGG, WAV
  - Documents: PDF, TXT, ZIP, RAR, 7Z

---

## WebSocket (Real-time)

Connect to `/ws?token=<jwt>` to establish a persistent connection.

### Client Messages (Events)
The client sends JSON messages in the format: `{"type": "EVENT_NAME", "data": { ... }}`

- `SUBSCRIBE`: Subscribe to events for a specific channel.
- `TYPING_START`: Indicate the user has started typing.
- `PRESENCE_UPDATE`: Manually update presence status.

### Server Events
The server broadcasts events to relevant connected clients.

- `READY`: Initial event upon connection, contains user session info.
- `MESSAGE_CREATE`: A new message was sent.
- `MESSAGE_UPDATE`: A message was edited, pinned, or reactions changed.
- `MESSAGE_DELETE`: A message was removed.
- `TYPING_START`: A user is typing in a channel.
- `PRESENCE_UPDATE`: A user's online status changed.
- `CHANNEL_CREATE/UPDATE/DELETE`: Channel-related changes.
- `COMMUNITY_UPDATE`: Community configuration changes.

