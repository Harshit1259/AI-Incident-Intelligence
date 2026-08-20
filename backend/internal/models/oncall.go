package models

import "time"

type OnCallSchedule struct {
	ID           string         `json:"id"`
	TenantID     string         `json:"tenant_id"`
	TeamName     string         `json:"team_name"`
	Timezone     string         `json:"timezone"`
	RotationType string         `json:"rotation_type"` // weekly, daily, custom
	Members      []OnCallMember `json:"members"`
	CreatedAt    time.Time      `json:"created_at"`
}

type OnCallMember struct {
	ID         int    `json:"id"`
	ScheduleID string `json:"schedule_id"`
	UserName   string `json:"user_name"`
	UserEmail  string `json:"user_email"`
	Position   int    `json:"position"`
}

type OnCallOverride struct {
	ID           int       `json:"id"`
	ScheduleID   string    `json:"schedule_id"`
	OriginalUser string    `json:"original_user"`
	OverrideUser string    `json:"override_user"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	Reason       string    `json:"reason"`
}

type OnCallCurrent struct {
	Schedule     OnCallSchedule  `json:"schedule"`
	CurrentUser  OnCallMember    `json:"current_user"`
	NextUser     OnCallMember    `json:"next_user"`
	NextRotation time.Time       `json:"next_rotation"`
	Override     *OnCallOverride `json:"override,omitempty"`
}

type OnCallCreateRequest struct {
	TeamName     string `json:"team_name"`
	Timezone     string `json:"timezone"`
	RotationType string `json:"rotation_type"`
	Members      []struct {
		UserName  string `json:"user_name"`
		UserEmail string `json:"user_email"`
	} `json:"members"`
}

type OnCallOverrideRequest struct {
	OverrideUser string `json:"override_user"`
	StartTime    string `json:"start_time"`
	EndTime      string `json:"end_time"`
	Reason       string `json:"reason"`
}
