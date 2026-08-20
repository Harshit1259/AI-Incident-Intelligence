package services

import (
	"fmt"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

type OnCallService struct {
	store *store.OnCallStore
}

func NewOnCallService(s *store.OnCallStore) *OnCallService {
	return &OnCallService{store: s}
}

// GetCurrentOnCall returns who is currently on-call for a schedule.
func (s *OnCallService) GetCurrentOnCall(scheduleID string) (*models.OnCallCurrent, error) {
	sched, err := s.store.GetScheduleByID(scheduleID)
	if err != nil {
		return nil, err
	}
	if sched == nil {
		return nil, fmt.Errorf("schedule not found: %s", scheduleID)
	}

	if len(sched.Members) == 0 {
		return &models.OnCallCurrent{
			Schedule:     *sched,
			NextRotation: time.Now(),
		}, nil
	}

	now := time.Now()

	// Load timezone
	loc, err := time.LoadLocation(sched.Timezone)
	if err != nil {
		loc = time.UTC
	}
	nowLocal := now.In(loc)

	// Check for active override
	overrides, err := s.store.GetActiveOverrides(scheduleID, now)
	if err != nil {
		return nil, err
	}

	memberCount := len(sched.Members)

	// Compute rotation position
	var currentPos int
	var nextRotation time.Time

	epoch := time.Date(2024, 1, 1, 0, 0, 0, 0, loc)

	switch sched.RotationType {
	case "daily":
		daysSinceEpoch := int(nowLocal.Sub(epoch).Hours() / 24)
		currentPos = daysSinceEpoch % memberCount
		// Next rotation: start of next day in schedule timezone
		nextDay := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day()+1, 0, 0, 0, 0, loc)
		nextRotation = nextDay
	default: // weekly
		weeksSinceEpoch := int(nowLocal.Sub(epoch).Hours() / (24 * 7))
		currentPos = weeksSinceEpoch % memberCount
		// Next rotation: start of next Monday in schedule timezone
		daysUntilMonday := (8 - int(nowLocal.Weekday())) % 7
		if daysUntilMonday == 0 {
			daysUntilMonday = 7
		}
		nextMonday := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day()+daysUntilMonday, 0, 0, 0, 0, loc)
		nextRotation = nextMonday
	}

	currentUser := sched.Members[currentPos]
	nextUser := sched.Members[(currentPos+1)%memberCount]

	result := &models.OnCallCurrent{
		Schedule:     *sched,
		CurrentUser:  currentUser,
		NextUser:     nextUser,
		NextRotation: nextRotation,
	}

	// If there's an active override, use the override user as current
	if len(overrides) > 0 {
		override := overrides[0]
		result.Override = &override
		// Find the override user in members or create a synthetic entry
		for _, m := range sched.Members {
			if m.UserName == override.OverrideUser {
				result.CurrentUser = m
				break
			}
		}
		// If not found in members, set synthetic
		if result.CurrentUser.UserName != override.OverrideUser {
			result.CurrentUser = models.OnCallMember{
				ScheduleID: scheduleID,
				UserName:   override.OverrideUser,
			}
		}
	}

	return result, nil
}
