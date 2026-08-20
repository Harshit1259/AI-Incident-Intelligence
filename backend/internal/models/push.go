package models

import "time"

// PushSubscription is one registered device for push notifications.
type PushSubscription struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	TenantID    string    `json:"tenant_id"`
	Platform    string    `json:"platform"`    // "expo" or "web"
	DeviceToken string    `json:"device_token"` // Expo token or Web Push endpoint
	P256DH      string    `json:"p256dh,omitempty"`
	AuthKey     string    `json:"auth_key,omitempty"`
	AppVersion  string    `json:"app_version"`
	CreatedAt   time.Time `json:"created_at"`
}

// RegisterDeviceRequest is the JSON body for POST /api/v1/push/register.
type RegisterDeviceRequest struct {
	Platform    string `json:"platform"`    // "expo" or "web"
	DeviceToken string `json:"device_token"` // Expo push token or Web Push endpoint
	P256DH      string `json:"p256dh,omitempty"`
	AuthKey     string `json:"auth_key,omitempty"`
	AppVersion  string `json:"app_version"`
}

// PushNotification is the message payload sent to devices.
type PushNotification struct {
	Title    string            `json:"title"`
	Body     string            `json:"body"`
	Sound    string            `json:"sound,omitempty"` // "default" for Expo
	Badge    int               `json:"badge,omitempty"`
	Data     map[string]string `json:"data,omitempty"`
}
