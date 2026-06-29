package admin

import (
	"time"

	"cotton-id/internal/audit"
	"cotton-id/internal/auth"
)

// This file defines the camelCase JSON response shapes the admin console returns
// (build-contract §3). They are intentionally separate from auth.PublicUser: the
// admin views expose operator-relevant fields (status, joinedAt, servicesCount)
// that the public profile projection deliberately omits.

// adminUserSummary is the per-user row in the listing + recent-signups + detail.
type adminUserSummary struct {
	ID            string    `json:"id"`
	DisplayName   string    `json:"displayName"`
	Username      string    `json:"username"`
	Email         string    `json:"email"`
	Status        string    `json:"status"`
	Role          string    `json:"role"`
	JoinedAt      time.Time `json:"joinedAt"`
	ServicesCount int       `json:"servicesCount"`
}

func toUserSummary(u UserListItem) adminUserSummary {
	return adminUserSummary{
		ID:            u.ID.String(),
		DisplayName:   u.DisplayName,
		Username:      u.Username,
		Email:         u.Email,
		Status:        u.Status,
		Role:          u.Role,
		JoinedAt:      u.JoinedAt,
		ServicesCount: u.ServicesCount,
	}
}

// adminUserDetail is the full per-user profile the detail view returns.
type adminUserDetail struct {
	ID            string    `json:"id"`
	DisplayName   string    `json:"displayName"`
	Username      string    `json:"username"`
	Email         string    `json:"email"`
	EmailVerified bool      `json:"emailVerified"`
	Status        string    `json:"status"`
	Role          string    `json:"role"`
	About         string    `json:"about"`
	Location      string    `json:"location"`
	JoinedAt      time.Time `json:"joinedAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

func toUserDetail(u *auth.User) adminUserDetail {
	return adminUserDetail{
		ID:            u.ID.String(),
		DisplayName:   u.DisplayName,
		Username:      u.Username,
		Email:         u.Email,
		EmailVerified: u.EmailVerified,
		Status:        u.Status,
		Role:          u.Role,
		About:         u.About,
		Location:      u.Location,
		JoinedAt:      u.CreatedAt,
		UpdatedAt:     u.UpdatedAt,
	}
}

// sessionView is a target user's active session in the detail view. No raw token
// or session id is ever exposed; only descriptive fields the operator can read.
type sessionView struct {
	UserAgent  string    `json:"userAgent"`
	IP         string    `json:"ip"`
	CreatedAt  time.Time `json:"createdAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
}

// auditView is one Journal/activity row. It mirrors audit.Entry but with the
// id/actor as strings for the wire.
type auditView struct {
	ID         string         `json:"id"`
	Timestamp  time.Time      `json:"ts"`
	ActorID    string         `json:"actorId,omitempty"`
	ActorLabel string         `json:"actorLabel,omitempty"`
	Action     string         `json:"action"`
	TargetType string         `json:"targetType,omitempty"`
	TargetID   string         `json:"targetId,omitempty"`
	IP         string         `json:"ip,omitempty"`
	RequestID  string         `json:"requestId,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

func toAuditView(e audit.Entry) auditView {
	v := auditView{
		ID:         e.ID.String(),
		Timestamp:  e.Timestamp,
		ActorLabel: e.ActorLabel,
		Action:     e.Action,
		TargetType: e.TargetType,
		TargetID:   e.TargetID,
		IP:         e.IP,
		RequestID:  e.RequestID,
		Metadata:   e.Metadata,
	}
	if e.ActorID != nil {
		v.ActorID = e.ActorID.String()
	}
	return v
}

func toAuditViews(es []audit.Entry) []auditView {
	out := make([]auditView, 0, len(es))
	for _, e := range es {
		out = append(out, toAuditView(e))
	}
	return out
}

// signupPointView is one day of the 30-day sign-up series. date is YYYY-MM-DD.
type signupPointView struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// --- top-level response envelopes ---

// overviewResponse is GET /admin/overview.
type overviewResponse struct {
	TotalUsers     int                `json:"totalUsers"`
	ActiveToday    int                `json:"activeToday"`
	NewThisWeek    int                `json:"newThisWeek"`
	Services       int                `json:"services"`
	Signups        []signupPointView  `json:"signups"`
	RecentSignups  []adminUserSummary `json:"recentSignups"`
	RecentActivity []auditView        `json:"recentActivity"`
}

// usersResponse is GET /admin/users (a page of the listing).
type usersResponse struct {
	Users    []adminUserSummary `json:"users"`
	Total    int                `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"pageSize"`
}

// userDetailResponse is GET /admin/users/{id}.
type userDetailResponse struct {
	User           adminUserDetail `json:"user"`
	Sessions       []sessionView   `json:"sessions"`
	RecentActivity []auditView     `json:"recentActivity"`
	Connections    int             `json:"connections"`
}

// userEnvelope wraps a single user in a {user} object for action responses.
type userEnvelope struct {
	User adminUserDetail `json:"user"`
}

// auditResponse is GET /admin/audit (a page of the Journal).
type auditResponse struct {
	Entries  []auditView `json:"entries"`
	Total    int         `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"pageSize"`
}

// changeRoleRequest is the PATCH /admin/users/{id}/role body.
type changeRoleRequest struct {
	Role string `json:"role" example:"admin"`
}

// messageUserRequest is the POST /admin/users/{id}/message body: an optional
// subject and a required (non-empty) body, emailed to the target user.
type messageUserRequest struct {
	Subject string `json:"subject" example:"A note from the cotton-id team"`
	Body    string `json:"body" example:"Hi — please verify your email address at your convenience."`
}

// messageResponse is a generic {message} acknowledgement (e.g. force reset).
type messageResponse struct {
	Message string `json:"message" example:"A password reset link has been issued."`
}
