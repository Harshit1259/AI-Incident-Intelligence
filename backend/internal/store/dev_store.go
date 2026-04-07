package store

import "database/sql"

type DevStore struct {
	db *sql.DB
}

func NewDevStore(db *sql.DB) *DevStore {
	return &DevStore{db: db}
}

func (devStore *DevStore) ResetAll() error {
	queries := []string{
		"DELETE FROM incident_events",
		"DELETE FROM incidents",
		"DELETE FROM events",
	}

	for _, query := range queries {
		if _, err := devStore.db.Exec(query); err != nil {
			return err
		}
	}

	return nil
}
