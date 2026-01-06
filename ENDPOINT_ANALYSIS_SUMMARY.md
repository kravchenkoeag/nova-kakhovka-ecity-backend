# ENDPOINT ANALYSIS SUMMARY
## Nova Kakhovka eCity Platform - Backend API

**Generated**: 2026-01-05  
**Purpose**: Complete API reference for frontend implementation  
**Base URL**: `http://localhost:8080` (development)  
**API Version**: v1  
**API Base Path**: `/api/v1`

---

## TABLE OF CONTENTS

1. [Authentication](#authentication)
2. [Authorization](#authorization)
3. [Base Configuration](#base-configuration)
4. [Public Endpoints](#public-endpoints)
5. [Protected Endpoints](#protected-endpoints)
6. [Moderator Endpoints](#moderator-endpoints)
7. [Admin Endpoints](#admin-endpoints)
8. [WebSocket Endpoints](#websocket-endpoints)
9. [Data Models](#data-models)
10. [Error Handling](#error-handling)
11. [Pagination](#pagination)
12. [Rate Limiting](#rate-limiting)

---

## AUTHENTICATION

### Authentication Method
- **Type**: JWT (JSON Web Tokens)
- **Header Format**: `Authorization: Bearer <token>`
- **Token Location**: Response body after login/register

### Token Structure
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": { /* User object */ }
}
```

### Storing Token
- Store token in localStorage or secure storage
- Include in `Authorization` header for all protected requests
- Token expiration: Configurable (default: 24 hours)

---

## AUTHORIZATION

### User Roles
- `USER` - Default role, basic permissions
- `MODERATOR` - Can moderate content, manage issues
- `ADMIN` - Full system access, user management
- `SUPER_ADMIN` - Highest level access

### Permission System
The backend uses a role-based permission system. Check user role before making requests.

---

## BASE CONFIGURATION

### CORS
- **Allowed Origins**:
  - `http://localhost:3000` (Next.js web)
  - `http://localhost:3001` (Next.js admin)
  - `https://ecity.gov.ua` (Production web)
  - `https://admin.ecity.gov.ua` (Production admin)
- **Allowed Methods**: GET, POST, PUT, PATCH, DELETE, OPTIONS
- **Allowed Headers**: Origin, Content-Type, Accept, Authorization, X-Requested-With
- **Credentials**: Enabled

### Headers
All requests should include:
```
Content-Type: application/json
Authorization: Bearer <token> (for protected endpoints)
```

---

## PUBLIC ENDPOINTS

No authentication required.

### 1. Health Check
```
GET /health
```
**Response**:
```json
{
  "status": "ok",
  "service": "nova-kakhovka-ecity",
  "version": "1.0.0",
  "time": "2026-01-05T12:00:00Z"
}
```

### 2. Authentication

#### Register User
```
POST /api/v1/auth/register
```

**Request Body**:
```json
{
  "email": "user@example.com",
  "password": "password123",
  "first_name": "John",
  "last_name": "Doe",
  "phone": "+380501234567"
}
```

**Validation**:
- `email`: Required, valid email format
- `password`: Required, min 6, max 100 characters
- `first_name`: Required, min 2, max 50 characters
- `last_name`: Required, min 2, max 50 characters
- `phone`: Optional

**Response** (201 Created):
```json
{
  "token": "jwt_token_here",
  "user": {
    "id": "507f1f77bcf86cd799439011",
    "email": "user@example.com",
    "first_name": "John",
    "last_name": "Doe",
    "role": "USER",
    "is_verified": false,
    "is_blocked": false,
    "created_at": "2026-01-05T12:00:00Z"
  }
}
```

**Error Responses**:
- `400` - Invalid request data
- `409` - User with this email already exists
- `500` - Server error

#### Login User
```
POST /api/v1/auth/login
```

**Request Body**:
```json
{
  "email": "user@example.com",
  "password": "password123"
}
```

**Response** (200 OK):
```json
{
  "token": "jwt_token_here",
  "user": { /* User object */ }
}
```

**Error Responses**:
- `400` - Invalid request data
- `401` - Invalid credentials
- `403` - Account is blocked (includes `block_reason`, `blocked_at`)

---

### 3. Groups (Public)

#### Get Public Groups
```
GET /api/v1/groups/public
```

**Query Parameters**:
- `page` (optional, default: 1)
- `limit` (optional, default: 20)

**Response** (200 OK):
```json
[
  {
    "id": "507f1f77bcf86cd799439011",
    "name": "Group Name",
    "description": "Group description",
    "type": "city",
    "is_public": true,
    "member_count": 150,
    "created_at": "2026-01-05T12:00:00Z"
  }
]
```

#### Search Groups
```
GET /api/v1/search/groups?q=search&type=city&limit=20
```

**Query Parameters**:
- `q` (optional) - Search query
- `type` (optional) - Group type filter
- `limit` (optional, default: 20, max: 50)

**Response** (200 OK):
```json
{
  "groups": [ /* Array of groups */ ],
  "count": 10
}
```

---

### 4. Announcements (Public)

#### Get All Announcements
```
GET /api/v1/announcements
```

**Query Parameters**:
- `category` (optional) - `work`, `help`, `services`, `housing`, `transport`
- `employment` (optional) - `once`, `permanent`, `partial`
- `created_from` (optional) - ISO 8601 date
- `created_to` (optional) - ISO 8601 date
- `page` (optional, default: 1)
- `limit` (optional, default: 20, max: 100)
- `sort_by` (optional) - `created_at`, `views`, `title`
- `sort_order` (optional) - `asc`, `desc`

**Response** (200 OK):
```json
{
  "announcements": [ /* Array of announcements */ ],
  "pagination": {
    "page": 1,
    "limit": 20,
    "total": 100,
    "total_pages": 5
  }
}
```

#### Get Announcement by ID
```
GET /api/v1/announcements/:id
```

**Response** (200 OK):
```json
{
  "id": "507f1f77bcf86cd799439011",
  "title": "Job Announcement",
  "description": "Detailed description",
  "category": "work",
  "employment": "permanent",
  "location": {
    "type": "Point",
    "coordinates": [46.7666, 33.3674],
    "address": "Main Street 1"
  },
  "contact_info": [
    {
      "type": "phone",
      "value": "+380501234567"
    }
  ],
  "is_active": true,
  "is_verified": true,
  "view_count": 42,
  "created_at": "2026-01-05T12:00:00Z"
}
```

---

### 5. Events (Public)

#### Get All Events
```
GET /api/v1/events
```

**Query Parameters**:
- `start_date` (optional) - ISO 8601 date
- `end_date` (optional) - ISO 8601 date
- `is_online` (optional) - `true`, `false`
- `is_public` (optional, default: `true`) - `true`, `false`
- `location` (optional)
- `organizer` (optional) - User ID
- `page` (optional, default: 1)
- `limit` (optional, default: 20, max: 50)
- `sort_by` (optional) - `start_date`, `created_at`, `participants_count`
- `sort_order` (optional) - `asc`, `desc`

**Response** (200 OK):
```json
{
  "events": [ /* Array of events */ ],
  "pagination": {
    "page": 1,
    "limit": 20,
    "total": 50,
    "total_pages": 3
  }
}
```

#### Get Event by ID
```
GET /api/v1/events/:id
```

#### Get Nearby Events
```
GET /api/v1/events/nearby?lat=46.7666&lng=33.3674&radius=5000
```

**Query Parameters**:
- `lat` (required) - Latitude
- `lng` (required) - Longitude
- `radius` (optional, default: 5000) - Radius in meters

**Response** (200 OK):
```json
{
  "events": [ /* Array of events */ ],
  "count": 5,
  "location": {
    "lat": 46.7666,
    "lng": 33.3674,
    "radius": 5000
  }
}
```

#### Search Events
```
GET /api/v1/search/events?q=search&category=social&date_from=2026-01-01
```

**Query Parameters**:
- `q` (optional) - Search query
- `category` (optional)
- `date_from` (optional) - YYYY-MM-DD format
- `page` (optional, default: 1)
- `limit` (optional, default: 20, max: 50)

---

### 6. Petitions (Public)

#### Get All Petitions
```
GET /api/v1/petitions
```

**Query Parameters**:
- `category` (optional) - `infrastructure`, `social`, `environment`, `economy`, `governance`, `safety`, `transport`, `education`, `healthcare`
- `status` (optional) - `draft`, `active`, `completed`, `under_review`, `accepted`, `rejected`
- `author_id` (optional) - User ID
- `date_from` (optional) - ISO 8601 date
- `date_to` (optional) - ISO 8601 date
- `min_signatures` (optional)
- `max_signatures` (optional)
- `tags` (optional) - Array of tags
- `goal_reached` (optional) - `true`, `false`
- `page` (optional, default: 1)
- `limit` (optional, default: 20, max: 50)
- `sort_by` (optional) - `created_at`, `signature_count`, `end_date`
- `sort_order` (optional) - `asc`, `desc`
- `search` (optional) - Text search

**Response** (200 OK):
```json
{
  "petitions": [ /* Array of petitions */ ],
  "pagination": {
    "page": 1,
    "limit": 20,
    "total": 30,
    "total_pages": 2
  }
}
```

#### Get Petition by ID
```
GET /api/v1/petitions/:id
```

**Response** (200 OK):
```json
{
  "id": "507f1f77bcf86cd799439011",
  "title": "Petition Title",
  "description": "Detailed description",
  "category": "infrastructure",
  "required_signatures": 100,
  "signature_count": 75,
  "demands": "Specific demands",
  "status": "active",
  "start_date": "2026-01-01T00:00:00Z",
  "end_date": "2026-02-01T00:00:00Z",
  "tags": ["tag1", "tag2"],
  "view_count": 500,
  "created_at": "2026-01-01T00:00:00Z"
}
```

---

### 7. Polls (Public)

#### Get All Polls
```
GET /api/v1/polls
```

**Query Parameters**:
- `status` (optional) - `draft`, `active`, `completed`, `closed`
- `category` (optional)
- `creator_id` (optional) - User ID
- `tag` (optional)
- `is_public` (optional) - `true`, `false`
- `page` (optional, default: 1)
- `limit` (optional, default: 10, max: 100)
- `sort_by` (optional)
- `sort_order` (optional) - `asc`, `desc`

**Response** (200 OK):
```json
{
  "polls": [ /* Array of polls */ ],
  "pagination": {
    "page": 1,
    "limit": 10,
    "total": 25,
    "total_pages": 3
  }
}
```

#### Get Poll by ID
```
GET /api/v1/polls/:id
```

#### Get Poll Results
```
GET /api/v1/polls/:id/results
```

**Response** (200 OK):
```json
{
  "poll_id": "507f1f77bcf86cd799439011",
  "title": "Poll Title",
  "total_responses": 150,
  "questions": [
    {
      "question_id": "507f1f77bcf86cd799439012",
      "text": "Question text",
      "type": "single_choice",
      "total_answers": 150,
      "options": [
        {
          "option_id": "507f1f77bcf86cd799439013",
          "text": "Option 1",
          "votes": 75,
          "percentage": "50.00"
        }
      ]
    }
  ]
}
```

---

### 8. City Issues (Public)

#### Get All City Issues
```
GET /api/v1/city-issues
```

**Query Parameters**:
- `category` (optional) - `road`, `lighting`, `water`, `electricity`, `waste`, `transport`, `building`, `safety`, `other`
- `status` (optional) - `reported`, `in_progress`, `resolved`, `rejected`, `duplicate`
- `priority` (optional) - `low`, `medium`, `high`, `critical`
- `reporter_id` (optional) - User ID
- `assigned_to` (optional)
- `date_from` (optional) - ISO 8601 date
- `date_to` (optional) - ISO 8601 date
- `is_verified` (optional) - `true`, `false`
- `bounds` (optional) - `lat1,lng1,lat2,lng2`
- `page` (optional, default: 1)
- `limit` (optional, default: 20, max: 100)
- `sort_by` (optional)
- `sort_order` (optional) - `asc`, `desc`

**Response** (200 OK):
```json
{
  "issues": [ /* Array of city issues */ ],
  "pagination": {
    "page": 1,
    "limit": 20,
    "total": 200,
    "total_pages": 10
  }
}
```

#### Get City Issue by ID
```
GET /api/v1/city-issues/:id
```

---

### 9. Transport (Public)

#### Get All Routes
```
GET /api/v1/transport/routes
```

**Query Parameters**:
- `type` (optional) - `bus`, `trolleybus`, `tram`
- `is_active` (optional) - `true`, `false`
- `near_location` (optional) - `lat,lng,radius_km`
- `search` (optional)
- `page` (optional, default: 1)
- `limit` (optional, default: 20, max: 100)

**Response** (200 OK):
```json
{
  "routes": [ /* Array of transport routes */ ],
  "pagination": {
    "page": 1,
    "limit": 20,
    "total": 15,
    "total_pages": 1
  }
}
```

#### Get Route by ID
```
GET /api/v1/transport/routes/:id
```

#### Get Nearby Stops
```
GET /api/v1/transport/stops/nearby?lat=46.7666&lng=33.3674&radius=1
```

**Query Parameters**:
- `lat` (required) - Latitude
- `lng` (required) - Longitude
- `radius` (optional, default: 1) - Radius in kilometers

**Response** (200 OK):
```json
{
  "stops": [
    {
      "_id": "Stop Name",
      "location": {
        "type": "Point",
        "coordinates": [33.3674, 46.7666]
      },
      "distance": 0.5,
      "routes": [
        {
          "route_id": "507f1f77bcf86cd799439011",
          "route_number": "1",
          "route_type": "bus",
          "route_name": "Route Name"
        }
      ]
    }
  ],
  "count": 5,
  "location": {
    "lat": 46.7666,
    "lng": 33.3674,
    "radius": 1
  }
}
```

#### Get Arrivals
```
GET /api/v1/transport/arrivals?stop=Stop%20Name&route=1&limit=10
```

**Query Parameters**:
- `stop` (required) - Stop name
- `route` (optional) - Route number
- `limit` (optional, default: 10, max: 50)

**Response** (200 OK):
```json
{
  "stop": "Stop Name",
  "arrivals": [
    {
      "route_number": "1",
      "route_name": "Route Name",
      "route_type": "bus",
      "vehicle_number": "123",
      "estimated_time": "2026-01-05T12:15:00Z",
      "minutes_away": 5,
      "stop_name": "Stop Name"
    }
  ],
  "count": 3,
  "time": "2026-01-05T12:10:00Z"
}
```

#### Get Live Tracking
```
GET /api/v1/transport/live?route_id=507f1f77bcf86cd799439011&bounds=lat1,lng1,lat2,lng2
```

**Query Parameters**:
- `route_id` (optional) - Route ID
- `bounds` (optional) - `lat1,lng1,lat2,lng2` for bounding box

**Response** (200 OK):
```json
{
  "live_vehicles": [
    {
      "vehicle_id": "507f1f77bcf86cd799439011",
      "vehicle_number": "123",
      "type": "bus",
      "route_id": "507f1f77bcf86cd799439012",
      "route_number": "1",
      "route_name": "Route Name",
      "location": {
        "type": "Point",
        "coordinates": [33.3674, 46.7666]
      },
      "speed": 40,
      "heading": 90,
      "last_update": "2026-01-05T12:10:00Z"
    }
  ],
  "count": 5,
  "timestamp": "2026-01-05T12:10:00Z"
}
```

---

### 10. Notifications (Public)

#### Get Notification Types
```
GET /api/v1/notification-types
```

**Response** (200 OK):
```json
{
  "notification_types": [
    "message",
    "event",
    "announcement",
    "system",
    "emergency"
  ]
}
```

---

## PROTECTED ENDPOINTS

Require `Authorization: Bearer <token>` header.

### 1. User Profile

#### Get User Profile
```
GET /api/v1/auth/profile
```

**Response** (200 OK):
```json
{
  "id": "507f1f77bcf86cd799439011",
  "email": "user@example.com",
  "first_name": "John",
  "last_name": "Doe",
  "phone": "+380501234567",
  "role": "USER",
  "is_verified": false,
  "is_blocked": false,
  "interests": ["technology", "sports"],
  "groups": ["507f1f77bcf86cd799439012"],
  "created_at": "2026-01-05T12:00:00Z"
}
```

#### Update User Profile
```
PUT /api/v1/auth/profile
```

**Request Body**:
```json
{
  "first_name": "John",
  "last_name": "Doe",
  "phone": "+380501234567",
  "interests": ["technology", "sports"],
  "profession": "Developer",
  "registered_address": "Main Street 1"
}
```

**Note**: Cannot update `email`, `password_hash`, `role`, `is_moderator`, `is_blocked`, `is_verified` through this endpoint.

**Response** (200 OK):
```json
{
  "message": "Profile updated successfully"
}
```

#### Change Password
```
PUT /api/v1/auth/password
```

**Request Body**:
```json
{
  "old_password": "oldpass123",
  "new_password": "newpass123"
}
```

**Validation**:
- `old_password`: Required
- `new_password`: Required, min 6 characters

**Response** (200 OK):
```json
{
  "message": "Password changed successfully"
}
```

---

### 2. Groups (Protected)

#### Create Group
```
POST /api/v1/groups
```

**Request Body**:
```json
{
  "name": "Group Name",
  "description": "Group description",
  "type": "city",
  "location_filter": "Nova Kakhovka",
  "interest_filter": ["technology"],
  "is_public": true,
  "auto_join": false,
  "max_members": 100
}
```

**Validation**:
- `name`: Required, min 3, max 100 characters
- `type`: Required, one of: `country`, `region`, `city`, `interest`
- `description`: Optional, max 500 characters

**Response** (201 Created):
```json
{
  "id": "507f1f77bcf86cd799439011",
  "name": "Group Name",
  "description": "Group description",
  "type": "city",
  "members": ["507f1f77bcf86cd799439012"],
  "admins": ["507f1f77bcf86cd799439012"],
  "is_public": true,
  "created_at": "2026-01-05T12:00:00Z"
}
```

#### Get User Groups
```
GET /api/v1/groups
```

**Response** (200 OK):
```json
[
  {
    "id": "507f1f77bcf86cd799439011",
    "name": "Group Name",
    "description": "Group description",
    "type": "city",
    "members": ["507f1f77bcf86cd799439012"],
    "is_public": true,
    "created_at": "2026-01-05T12:00:00Z"
  }
]
```

#### Get Group by ID
```
GET /api/v1/groups/:id
```

**Note**: Must be a member to access private groups.

#### Update Group
```
PUT /api/v1/groups/:id
```

**Request Body**:
```json
{
  "name": "Updated Name",
  "description": "Updated description",
  "is_public": true
}
```

**Note**: Only group creator can update.

#### Delete Group
```
DELETE /api/v1/groups/:id
```

**Note**: Only group creator can delete. Deletes all messages.

#### Join Group
```
POST /api/v1/groups/:id/join
```

**Response** (200 OK):
```json
{
  "message": "Successfully joined group"
}
```

#### Leave Group
```
POST /api/v1/groups/:id/leave
```

**Note**: Creator cannot leave group.

**Response** (200 OK):
```json
{
  "message": "Successfully left the group"
}
```

#### Send Message to Group
```
POST /api/v1/groups/:id/messages
```

**Request Body**:
```json
{
  "content": "Message content",
  "type": "text",
  "media_url": "https://example.com/image.jpg",
  "reply_to_id": "507f1f77bcf86cd799439011"
}
```

**Validation**:
- `content`: Required, max 1000 characters
- `type`: Required, one of: `text`, `image`, `video`, `file`, `link`

**Response** (201 Created):
```json
{
  "id": "507f1f77bcf86cd799439011",
  "group_id": "507f1f77bcf86cd799439012",
  "user_id": "507f1f77bcf86cd799439013",
  "content": "Message content",
  "type": "text",
  "created_at": "2026-01-05T12:00:00Z"
}
```

#### Get Group Messages
```
GET /api/v1/groups/:id/messages
```

**Query Parameters**:
- `page` (optional, default: 1)
- `limit` (optional, default: 50)

**Response** (200 OK):
```json
[
  {
    "id": "507f1f77bcf86cd799439011",
    "group_id": "507f1f77bcf86cd799439012",
    "user_id": "507f1f77bcf86cd799439013",
    "content": "Message content",
    "type": "text",
    "is_edited": false,
    "is_deleted": false,
    "created_at": "2026-01-05T12:00:00Z"
  }
]
```

---

### 3. Announcements (Protected)

#### Create Announcement
```
POST /api/v1/announcements
```

**Request Body**:
```json
{
  "title": "Job Announcement",
  "description": "Detailed job description",
  "category": "work",
  "location": {
    "type": "Point",
    "coordinates": [33.3674, 46.7666],
    "address": "Main Street 1"
  },
  "address": "Main Street 1, Nova Kakhovka",
  "employment": "permanent",
  "contact_info": [
    {
      "type": "phone",
      "value": "+380501234567"
    }
  ],
  "media_files": ["https://example.com/image.jpg"],
  "expires_at": "2026-02-05T12:00:00Z"
}
```

**Validation**:
- `title`: Required, min 5, max 200 characters
- `description`: Required, min 10, max 2000 characters
- `category`: Required, one of: `work`, `help`, `services`, `housing`, `transport`
- `employment`: Optional, one of: `once`, `permanent`, `partial`
- `contact_info`: Required, min 1 item
- `expires_at`: Optional, defaults to 30 days from creation

**Limits**:
- Maximum 5 active announcements per user

**Response** (201 Created):
```json
{
  "id": "507f1f77bcf86cd799439011",
  "title": "Job Announcement",
  "description": "Detailed job description",
  "category": "work",
  "status": "pending",
  "is_verified": false,
  "is_active": true,
  "created_at": "2026-01-05T12:00:00Z",
  "expires_at": "2026-02-05T12:00:00Z"
}
```

#### Update Announcement
```
PUT /api/v1/announcements/:id
```

**Request Body**:
```json
{
  "title": "Updated Title",
  "description": "Updated description",
  "is_active": true
}
```

**Note**: Only author or moderator can update. Non-moderator updates reset verification status.

#### Delete Announcement
```
DELETE /api/v1/announcements/:id
```

**Note**: Only author or moderator can delete.

---

### 4. Events (Protected)

#### Create Event
```
POST /api/v1/events
```

**Request Body**:
```json
{
  "title": "City Event",
  "description": "Event description",
  "start_date": "2026-02-01T18:00:00Z",
  "end_date": "2026-02-01T22:00:00Z",
  "location": {
    "type": "Point",
    "coordinates": [33.3674, 46.7666],
    "address": "Main Square"
  },
  "address": "Main Square, Nova Kakhovka",
  "is_online": false,
  "max_participants": 100,
  "is_public": true
}
```

**Validation**:
- `title`: Required, min 5, max 200 characters
- `description`: Required, min 10, max 2000 characters
- `start_date`: Required, cannot be in the past
- `end_date`: Optional, must be after start_date

**Response** (201 Created):
```json
{
  "id": "507f1f77bcf86cd799439011",
  "title": "City Event",
  "description": "Event description",
  "organizer_id": "507f1f77bcf86cd799439012",
  "start_date": "2026-02-01T18:00:00Z",
  "participants": ["507f1f77bcf86cd799439012"],
  "created_at": "2026-01-05T12:00:00Z"
}
```

#### Update Event
```
PUT /api/v1/events/:id
```

**Note**: Only organizer can update.

#### Delete Event
```
DELETE /api/v1/events/:id
```

**Note**: Only organizer can delete.

#### Attend Event
```
POST /api/v1/events/:id/attend
```

**Response** (200 OK):
```json
{
  "message": "Successfully marked as attending"
}
```

#### Leave Event
```
POST /api/v1/events/:id/leave
```

**Note**: Organizer cannot leave their own event.

---

### 5. Petitions (Protected)

#### Create Petition
```
POST /api/v1/petitions
```

**Request Body**:
```json
{
  "title": "Petition Title",
  "description": "Detailed petition description",
  "category": "infrastructure",
  "required_signatures": 100,
  "demands": "Specific demands and requirements",
  "end_date": "2026-02-01T00:00:00Z",
  "tags": ["tag1", "tag2"],
  "attachment_urls": ["https://example.com/doc.pdf"]
}
```

**Validation**:
- `title`: Required, min 10, max 300 characters
- `description`: Required, min 50, max 5000 characters
- `category`: Required, one of: `infrastructure`, `social`, `environment`, `economy`, `governance`, `safety`, `transport`, `education`, `healthcare`
- `required_signatures`: Optional, min 100, defaults to 100
- `demands`: Required, min 20, max 2000 characters
- `end_date`: Required, must be at least 24 hours from now

**Limits**:
- Maximum 3 active petitions per user

**Response** (201 Created):
```json
{
  "id": "507f1f77bcf86cd799439011",
  "title": "Petition Title",
  "status": "draft",
  "signature_count": 0,
  "created_at": "2026-01-05T12:00:00Z",
  "end_date": "2026-02-01T00:00:00Z"
}
```

#### Sign Petition
```
POST /api/v1/petitions/:id/sign
```

**Request Body**:
```json
{
  "comment": "I support this petition",
  "diia_key_id": "optional_diia_key"
}
```

**Response** (201 Created):
```json
{
  "message": "Petition signed successfully",
  "signature_count": 76,
  "completed": false
}
```

#### Update Petition
```
PUT /api/v1/petitions/:id
```

**Note**: Only author or moderator can update.

---

### 6. Polls (Protected)

#### Create Poll
```
POST /api/v1/polls
```

**Request Body**:
```json
{
  "title": "Poll Title",
  "description": "Poll description",
  "category": "city_planning",
  "questions": [
    {
      "text": "Question text?",
      "type": "single_choice",
      "is_required": true,
      "options": [
        {"text": "Option 1"},
        {"text": "Option 2"}
      ]
    }
  ],
  "allow_multiple": false,
  "is_anonymous": false,
  "is_public": true,
  "target_groups": ["507f1f77bcf86cd799439011"],
  "start_date": "2026-01-05T12:00:00Z",
  "end_date": "2026-01-10T12:00:00Z",
  "tags": ["tag1"]
}
```

**Validation**:
- `title`: Required, min 5, max 300 characters
- `description`: Required, min 10, max 2000 characters
- `category`: Required, one of: `city_planning`, `transport`, `infrastructure`, `social`, `environment`, `governance`, `budget`, `education`, `healthcare`
- `questions`: Required, min 1, max 20 questions
- `end_date`: Required, must be after start_date
- Minimum poll duration: 1 hour

**Question Types**:
- `single_choice` - Requires options
- `multiple_choice` - Requires options
- `rating` - Requires min_rating, max_rating
- `text` - Optional max_length
- `scale` - Requires min/max values
- `yes_no` - Boolean answer

**Limits**:
- Maximum 5 active polls per user
- Rate limiting: 5 minutes between poll creation

**Response** (201 Created):
```json
{
  "id": "507f1f77bcf86cd799439011",
  "title": "Poll Title",
  "status": "active",
  "questions": [ /* Array of questions */ ],
  "created_at": "2026-01-05T12:00:00Z",
  "end_date": "2026-01-10T12:00:00Z"
}
```

#### Vote in Poll
```
POST /api/v1/polls/:id/respond
```

**Request Body**:
```json
{
  "answers": [
    {
      "question_id": "507f1f77bcf86cd799439012",
      "option_ids": ["507f1f77bcf86cd799439013"],
      "text_answer": null,
      "number_answer": null,
      "bool_answer": null
    }
  ]
}
```

**Validation**:
- All required questions must have answers
- Answer format must match question type

**Response** (200 OK):
```json
{
  "message": "Vote submitted successfully"
}
```

#### Update Poll
```
PUT /api/v1/polls/:id
```

**Note**: Only creator or moderator can update.

#### Delete Poll
```
DELETE /api/v1/polls/:id
```

**Note**: Only creator or moderator can delete.

---

### 7. City Issues (Protected)

#### Create City Issue
```
POST /api/v1/city-issues
```

**Request Body**:
```json
{
  "title": "Issue Title",
  "description": "Detailed issue description",
  "category": "road",
  "priority": "high",
  "location": {
    "type": "Point",
    "coordinates": [33.3674, 46.7666]
  },
  "address": "Main Street 1",
  "photos": ["https://example.com/photo.jpg"],
  "videos": ["https://example.com/video.mp4"]
}
```

**Validation**:
- `title`: Required, min 5, max 200 characters
- `description`: Required, min 10, max 1000 characters
- `category`: Required, one of: `road`, `lighting`, `water`, `electricity`, `waste`, `transport`, `building`, `safety`, `other`
- `priority`: Optional, one of: `low`, `medium`, `high`, `critical` (defaults to `medium`)
- `location`: Required
- `address`: Required

**Limits**:
- Maximum 10 active issues per user

**Response** (201 Created):
```json
{
  "id": "507f1f77bcf86cd799439011",
  "title": "Issue Title",
  "status": "reported",
  "priority": "high",
  "upvote_count": 0,
  "created_at": "2026-01-05T12:00:00Z"
}
```

#### Update City Issue
```
PUT /api/v1/city-issues/:id
```

**Request Body**:
```json
{
  "title": "Updated Title",
  "description": "Updated description"
}
```

**Note**: Only author can update.

#### Upvote City Issue
```
POST /api/v1/city-issues/:id/upvote
```

**Response** (200 OK):
```json
{
  "message": "Issue upvoted successfully"
}
```

---

### 8. Notifications (Protected)

#### Get Notifications
```
GET /api/v1/notifications
```

**Query Parameters**:
- `page` (optional, default: 1)
- `limit` (optional, default: 20, max: 100)
- `unread_only` (optional) - `true`, `false`
- `type` (optional) - Notification type filter

**Response** (200 OK):
```json
{
  "notifications": [
    {
      "id": "507f1f77bcf86cd799439011",
      "user_id": "507f1f77bcf86cd799439012",
      "title": "Notification Title",
      "body": "Notification body",
      "type": "system",
      "is_read": false,
      "data": {
        "action": "view_petition",
        "petition_id": "507f1f77bcf86cd799439013"
      },
      "created_at": "2026-01-05T12:00:00Z"
    }
  ],
  "unread_count": 5,
  "pagination": {
    "page": 1,
    "limit": 20,
    "total": 50,
    "total_pages": 3
  }
}
```

#### Mark Notification as Read
```
PUT /api/v1/notifications/:id/read
```

**Response** (200 OK):
```json
{
  "message": "Notification marked as read"
}
```

#### Mark All Notifications as Read
```
PUT /api/v1/notifications/read-all
```

**Response** (200 OK):
```json
{
  "message": "All notifications marked as read",
  "updated_count": 10
}
```

#### Delete Notification
```
DELETE /api/v1/notifications/:id
```

**Response** (200 OK):
```json
{
  "message": "Notification deleted successfully"
}
```

#### Register Device Token
```
POST /api/v1/device-tokens
```

**Request Body**:
```json
{
  "token": "fcm_token_here",
  "platform": "android",
  "device_id": "device_unique_id"
}
```

**Validation**:
- `token`: Required
- `platform`: Required, one of: `ios`, `android`, `web`

**Response** (201 Created):
```json
{
  "message": "Device token registered successfully"
}
```

#### Unregister Device Token
```
DELETE /api/v1/device-tokens/:token
```

**Response** (200 OK):
```json
{
  "message": "Device token unregistered successfully"
}
```

#### Get Notification Preferences
```
GET /api/v1/notification-preferences
```

**Response** (200 OK):
```json
{
  "preferences": {
    "email": true,
    "push": true,
    "sms": false,
    "in_app": true,
    "announcements": true,
    "events": true,
    "city_issues": true,
    "polls": true,
    "petitions": true
  }
}
```

#### Update Notification Preferences
```
PUT /api/v1/notification-preferences
```

**Request Body**:
```json
{
  "email": true,
  "push": false,
  "announcements": true
}
```

**Response** (200 OK):
```json
{
  "message": "Notification preferences updated successfully"
}
```

---

### 9. Search (Protected)

#### Search Users
```
GET /api/v1/search/users?q=search&limit=20
```

**Query Parameters**:
- `q` (optional) - Search query
- `limit` (optional, default: 20, max: 50)

**Response** (200 OK):
```json
{
  "users": [ /* Array of users */ ],
  "count": 10
}
```

---

### 10. Statistics (Protected)

#### Get User Statistics
```
GET /api/v1/stats/user
```

**Response** (200 OK):
```json
{
  "total_users": 1000,
  "verified_users": 750,
  "blocked_users": 5,
  "users_by_role": [
    {
      "_id": "USER",
      "count": 950
    }
  ],
  "new_users_last_month": 50,
  "new_users_last_week": 10,
  "timestamp": "2026-01-05T12:00:00Z"
}
```

#### Get Group Statistics
```
GET /api/v1/stats/groups/:id
```

**Response** (200 OK):
```json
{
  "group_id": "507f1f77bcf86cd799439011",
  "name": "Group Name",
  "member_count": 150,
  "message_count": 500,
  "created_at": "2026-01-05T12:00:00Z",
  "type": "city",
  "is_public": true
}
```

---

## MODERATOR ENDPOINTS

Require `Authorization: Bearer <token>` header and `MODERATOR` role.

### 1. Announcements

#### Approve Announcement
```
PUT /api/v1/announcements/:id/approve
```

**Response** (200 OK):
```json
{
  "message": "Announcement approved successfully"
}
```

#### Reject Announcement
```
PUT /api/v1/announcements/:id/reject
```

**Request Body**:
```json
{
  "reason": "Rejection reason"
}
```

**Validation**:
- `reason`: Required, min 10, max 500 characters

**Response** (200 OK):
```json
{
  "message": "Announcement rejected successfully"
}
```

#### Get Pending Announcements
```
GET /api/v1/moderation/posts/pending
```

**Response** (200 OK):
```json
{
  "announcements": [ /* Array of pending announcements */ ],
  "count": 5
}
```

---

### 2. Events

#### Moderate Event
```
PUT /api/v1/events/:id/moderate
```

**Request Body**:
```json
{
  "action": "approve",
  "reason": "Optional reason"
}
```

**Validation**:
- `action`: Required, one of: `approve`, `reject`

**Response** (200 OK):
```json
{
  "message": "Event moderated successfully",
  "status": "approved"
}
```

---

### 3. City Issues

#### Update Issue Status
```
PUT /api/v1/city-issues/:id/status
```

**Request Body**:
```json
{
  "status": "in_progress",
  "note": "Status change note"
}
```

**Validation**:
- `status`: Required, one of: `pending`, `reported`, `in_progress`, `resolved`, `rejected`

**Response** (200 OK):
```json
{
  "message": "Issue status updated successfully",
  "status": "in_progress"
}
```

#### Assign Issue
```
PUT /api/v1/city-issues/:id/assign
```

**Request Body**:
```json
{
  "assigned_to_id": "507f1f77bcf86cd799439011",
  "note": "Assignment note"
}
```

**Validation**:
- `assigned_to_id`: Required, valid User ID

**Response** (200 OK):
```json
{
  "message": "Issue assigned successfully",
  "assigned_to": "John Doe"
}
```

---

### 4. Polls

#### Update Poll Status (Moderator)
```
PUT /api/v1/polls/:id/status
```

**Note**: Same as regular update, but moderator can update any poll.

#### Force Delete Poll
```
DELETE /api/v1/polls/:id/force
```

**Note**: Moderator can delete any poll.

---

### 5. Petitions

#### Update Petition Status
```
PUT /api/v1/petitions/:id/status
```

**Request Body**:
```json
{
  "status": "open",
  "response": "Official response"
}
```

**Validation**:
- `status`: Optional, one of: `open`, `closed`, `under_review`, `approved`, `rejected`

---

### 6. User Management

#### Ban User
```
POST /api/v1/moderation/users/:id/ban
```

**Response** (200 OK):
```json
{
  "message": "User banned successfully",
  "user_id": "507f1f77bcf86cd799439011"
}
```

#### Unban User
```
POST /api/v1/moderation/users/:id/unban
```

**Response** (200 OK):
```json
{
  "message": "User unbanned successfully",
  "user_id": "507f1f77bcf86cd799439011"
}
```

---

### 7. Statistics

#### Get Platform Statistics
```
GET /api/v1/stats/platform
```

**Response** (200 OK):
```json
{
  "total_events": 150,
  "events_by_status": [
    {
      "_id": "approved",
      "count": 120
    }
  ],
  "popular_events": [ /* Array of popular events */ ],
  "timestamp": "2026-01-05T12:00:00Z"
}
```

---

## ADMIN ENDPOINTS

Require `Authorization: Bearer <token>` header and `ADMIN` role.

### 1. User Management

#### Get All Users
```
GET /api/v1/users
```

**Query Parameters**:
- `page` (optional, default: 1)
- `limit` (optional, default: 20, max: 100)
- `search` (optional) - Search by email or name
- `role` (optional) - Filter by role
- `is_blocked` (optional) - `true`, `false`

**Response** (200 OK):
```json
{
  "data": [ /* Array of users */ ],
  "users": [ /* Legacy format */ ],
  "total": 1000,
  "page": 1,
  "limit": 20,
  "total_pages": 50
}
```

#### Get User by ID
```
GET /api/v1/users/:id
```

**Response** (200 OK):
```json
{
  "user": { /* User object */ }
}
```

#### Update User
```
PUT /api/v1/users/:id
```

**Request Body**:
```json
{
  "full_name": "Full Name",
  "phone": "+380501234567",
  "date_of_birth": "1990-01-01",
  "gender": "male",
  "address": "Address"
}
```

#### Delete User
```
DELETE /api/v1/users/:id
```

**Note**: Soft delete (sets `is_deleted` and `is_blocked` flags).

#### Block User
```
PUT /api/v1/users/:id/block
```

**Request Body**:
```json
{
  "is_blocked": true,
  "reason": "Block reason"
}
```

**Response** (200 OK):
```json
{
  "message": "User status updated successfully",
  "user_id": "507f1f77bcf86cd799439011",
  "is_blocked": true
}
```

#### Unblock User
```
PUT /api/v1/users/:id/unblock
```

**Response** (200 OK):
```json
{
  "message": "User unblocked successfully"
}
```

#### Verify User
```
PUT /api/v1/users/:id/verify
```

**Response** (200 OK):
```json
{
  "message": "User verified successfully"
}
```

#### Update User Role
```
PUT /api/v1/users/:id/role
```

**Request Body**:
```json
{
  "role": "MODERATOR"
}
```

**Validation**:
- `role`: Required, one of: `USER`, `MODERATOR`, `ADMIN`

**Response** (200 OK):
```json
{
  "message": "User role updated successfully",
  "role": "MODERATOR"
}
```

---

### 2. Notifications

#### Send Notification
```
POST /api/v1/notifications/send
```

**Request Body**:
```json
{
  "user_ids": ["507f1f77bcf86cd799439011", "507f1f77bcf86cd799439012"],
  "title": "Notification Title",
  "body": "Notification body",
  "type": "system",
  "data": {
    "action": "view_event",
    "event_id": "507f1f77bcf86cd799439013"
  }
}
```

**Validation**:
- `user_ids`: Required, array of User IDs
- `title`: Required, max 100 characters
- `body`: Required, max 500 characters
- `type`: Required, one of: `message`, `event`, `announcement`, `system`, `emergency`

**Response** (200 OK):
```json
{
  "message": "Notification sent successfully",
  "user_count": 2
}
```

#### Send Emergency Notification
```
POST /api/v1/notifications/emergency
```

**Request Body**:
```json
{
  "title": "Emergency Alert",
  "body": "Emergency message to all users",
  "data": {
    "type": "emergency"
  }
}
```

**Note**: Sends to ALL users.

**Response** (200 OK):
```json
{
  "message": "Emergency notification sent to all users"
}
```

---

### 3. Transport Management

#### Create Route
```
POST /api/v1/transport/routes
```

**Request Body**:
```json
{
  "number": "1",
  "type": "bus",
  "name": "Route Name",
  "description": "Route description",
  "color": "#FF0000",
  "stops": [
    {
      "name": "Stop 1",
      "location": {
        "type": "Point",
        "coordinates": [33.3674, 46.7666]
      },
      "stop_order": 1
    }
  ],
  "route_points": [
    {
      "type": "Point",
      "coordinates": [33.3674, 46.7666]
    }
  ],
  "schedule": [],
  "is_active": true,
  "fare": 5.0
}
```

**Validation**:
- `number`: Required, unique per type
- `type`: Required, one of: `bus`, `trolleybus`, `tram`
- `stops`: Required, min 2 stops
- `route_points`: Required, min 2 points

#### Update Route
```
PUT /api/v1/transport/routes/:id
```

#### Delete Route
```
DELETE /api/v1/transport/routes/:id
```

**Note**: Cannot delete route with active vehicles.

#### Create Vehicle
```
POST /api/v1/transport/vehicles
```

**Request Body**:
```json
{
  "route_id": "507f1f77bcf86cd799439011",
  "vehicle_number": "123",
  "type": "bus",
  "model": "Mercedes-Benz",
  "capacity": 50,
  "current_location": {
    "type": "Point",
    "coordinates": [33.3674, 46.7666]
  },
  "is_active": true,
  "has_air_conditioner": true,
  "has_wifi": false,
  "is_accessible": true
}
```

**Validation**:
- `route_id`: Required, valid Route ID
- `vehicle_number`: Required, unique
- `type`: Required, one of: `bus`, `trolleybus`, `tram`

#### Update Vehicle
```
PUT /api/v1/transport/vehicles/:id
```

#### Delete Vehicle
```
DELETE /api/v1/transport/vehicles/:id
```

#### Update Vehicle Location
```
PUT /api/v1/transport/vehicles/:id/location
```

**Request Body**:
```json
{
  "location": {
    "type": "Point",
    "coordinates": [33.3674, 46.7666]
  },
  "speed": 40,
  "heading": 90
}
```

**Note**: For drivers/vehicle tracking systems.

---

### 4. Analytics

#### Get User Analytics
```
GET /api/v1/analytics/users
```

**Response** (200 OK):
```json
{
  "total_users": 1000,
  "verified_users": 750,
  "blocked_users": 5,
  "users_by_role": [ /* Array */ ],
  "new_users_last_month": 50,
  "new_users_last_week": 10,
  "timestamp": "2026-01-05T12:00:00Z"
}
```

#### Get Content Analytics
```
GET /api/v1/analytics/content
```

**Response** (200 OK):
```json
{
  "total_events": 150,
  "events_by_status": [ /* Array */ ],
  "popular_events": [ /* Array */ ],
  "timestamp": "2026-01-05T12:00:00Z"
}
```

#### Get Poll Analytics
```
GET /api/v1/analytics/polls
```

**Response** (200 OK):
```json
{
  "total_polls": 50,
  "total_responses": 1500,
  "average_responses": 30,
  "active_polls": 10,
  "completed_polls": 35,
  "recent_polls": 5,
  "polls_by_status": [ /* Array */ ],
  "polls_by_category": [ /* Array */ ],
  "popular_polls": [ /* Array */ ],
  "timestamp": "2026-01-05T12:00:00Z"
}
```

---

## WEBSOCKET ENDPOINTS

### WebSocket Connection
```
WS /ws?token=<jwt_token>&group_id=<group_id>
```

**Query Parameters**:
- `token` (required) - JWT token
- `group_id` (required) - Group ID to join

**Connection**:
- Use WebSocket library (e.g., `ws` for Node.js, native WebSocket for browsers)
- Connection remains open for real-time messaging

**Message Types**:

#### Send Message
```json
{
  "type": "send_message",
  "data": {
    "content": "Message content",
    "type": "text",
    "media_url": "https://example.com/image.jpg"
  }
}
```

#### Typing Indicator
```json
{
  "type": "typing",
  "group_id": "507f1f77bcf86cd799439011"
}
```

#### Ping
```json
{
  "type": "ping"
}
```

**Received Messages**:

#### New Message
```json
{
  "type": "new_message",
  "data": {
    "id": "507f1f77bcf86cd799439011",
    "group_id": "507f1f77bcf86cd799439012",
    "user_id": "507f1f77bcf86cd799439013",
    "content": "Message content",
    "type": "text",
    "created_at": "2026-01-05T12:00:00Z"
  }
}
```

#### User Typing
```json
{
  "type": "user_typing",
  "data": {
    "user_id": "507f1f77bcf86cd799439011",
    "group_id": "507f1f77bcf86cd799439012"
  }
}
```

#### Pong
```json
{
  "type": "pong"
}
```

---

## PAGINATION

Most list endpoints support pagination with the following structure:

**Query Parameters**:
- `page` (optional, default: 1) - Page number
- `limit` (optional, default: varies) - Items per page

**Response Format**:
```json
{
  "data": [ /* Array of items */ ],
  "pagination": {
    "page": 1,
    "limit": 20,
    "total": 100,
    "total_pages": 5
  }
}
```

**Some endpoints use different response formats**:
- Groups: Returns array directly
- Events: `events` key instead of `data`
- Announcements: `announcements` key instead of `data`

---

## ERROR HANDLING

### Error Response Format
```json
{
  "error": "Error message",
  "details": "Additional error details (optional)"
}
```

### HTTP Status Codes
- `200` - OK
- `201` - Created
- `400` - Bad Request (validation errors)
- `401` - Unauthorized (missing or invalid token)
- `403` - Forbidden (insufficient permissions)
- `404` - Not Found
- `409` - Conflict (e.g., duplicate email)
- `429` - Too Many Requests (rate limit exceeded)
- `500` - Internal Server Error

### Common Error Scenarios

#### Missing Authorization Header
```json
{
  "error": "Authorization header is required"
}
```

#### Invalid Token
```json
{
  "error": "Invalid token"
}
```

#### Validation Error
```json
{
  "error": "Invalid request data",
  "details": "email: required validation failed"
}
```

#### Account Blocked (Login)
```json
{
  "error": "Account is blocked",
  "is_blocked": true,
  "block_reason": "Violation of terms",
  "blocked_at": "2026-01-01T12:00:00Z",
  "message": "Ваш акаунт заблоковано. Будь ласка, зверніться до модератора для отримання додаткової інформації."
}
```

#### Rate Limit Exceeded
```json
{
  "error": "Rate limit exceeded",
  "details": "Please wait 4m30s before creating another poll"
}
```

---

## RATE LIMITING

### Current Rate Limits
- **Poll Creation**: 5 minutes between creations
- **Announcements**: Maximum 5 active announcements per user
- **Petitions**: Maximum 3 active petitions per user
- **City Issues**: Maximum 10 active issues per user
- **Polls**: Maximum 5 active polls per user

### Rate Limit Headers
Some endpoints may include rate limit headers (not currently implemented, but recommended):
```
X-RateLimit-Limit: 5
X-RateLimit-Remaining: 3
X-RateLimit-Reset: 1641394800
```

---

## DATA MODELS

### Location Object
```typescript
interface Location {
  type: "Point";
  coordinates: [number, number]; // [longitude, latitude]
  address?: string;
  city?: string;
}
```

### User Object
```typescript
interface User {
  id: string;
  email: string;
  first_name: string;
  last_name: string;
  phone?: string;
  avatar?: string;
  role: "USER" | "MODERATOR" | "ADMIN" | "SUPER_ADMIN";
  is_verified: boolean;
  is_blocked: boolean;
  block_reason?: string;
  blocked_at?: string;
  interests: string[];
  groups: string[];
  location?: Location;
  created_at: string;
  updated_at: string;
}
```

### Notification Preferences
```typescript
interface NotificationPreferences {
  email: boolean;
  push: boolean;
  sms: boolean;
  in_app: boolean;
  announcements: boolean;
  events: boolean;
  city_issues: boolean;
  polls: boolean;
  petitions: boolean;
}
```

### Contact Info
```typescript
interface ContactInfo {
  type: "phone" | "email" | "telegram" | "viber" | "other";
  value: string;
}
```

---

## IMPLEMENTATION NOTES

### Frontend Recommendations

1. **API Client Setup**:
   - Create base API client with automatic token injection
   - Implement request/response interceptors
   - Handle token refresh (if implemented)

2. **Error Handling**:
   - Create centralized error handler
   - Show user-friendly error messages
   - Handle 401/403 errors (redirect to login)

3. **Pagination**:
   - Implement reusable pagination component
   - Use infinite scroll or page-based navigation
   - Cache paginated results

4. **Real-time Updates**:
   - Use WebSocket for group messages
   - Implement reconnection logic
   - Show connection status

5. **File Upload**:
   - Announcements, events, and issues support media URLs
   - Implement file upload to storage service first
   - Send URLs in API requests

6. **Form Validation**:
   - Match backend validation rules
   - Show inline validation errors
   - Disable submit until valid

7. **Authentication Flow**:
   - Store token securely
   - Check token validity on app start
   - Implement auto-logout on token expiry

8. **Permission Checks**:
   - Check user role before showing admin/mod items
   - Disable actions user cannot perform
   - Show appropriate error messages



## VERSION HISTORY

- **v1.0.0** (2026-01-05) - Initial endpoint analysis

---

**Document Status**: Complete  
**Last Updated**: 2026-01-05  
**Maintained By**: Backend Development Team

