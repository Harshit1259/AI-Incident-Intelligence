package store


import (
	"context"
"time"
)
// UpdateSlackChannelID sets the slack_channel_id column for an incident
// without touching any other field.
func (s *IncidentStore) UpdateSlackChannelID(incidentID, channelID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`UPDATE incidents SET slack_channel_id = $2 WHERE id = $1`,
		incidentID, channelID,
	)
	return err
}

// GetSlackChannelID returns the Slack channel ID linked to an incident.
// Returns "" on any error or if the column is empty.
func (s *IncidentStore) GetSlackChannelID(incidentID string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var channelID string
	err := s.db.QueryRowContext(ctx, 
		`SELECT COALESCE(slack_channel_id, '') FROM incidents WHERE id = $1`,
		incidentID,
	).Scan(&channelID)
	if err != nil {
		return ""
	}
	return channelID
}
