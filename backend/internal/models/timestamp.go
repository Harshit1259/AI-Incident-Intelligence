package models

import "time"

// TimestampFormat is the canonical RFC 3339 / ISO-8601 format used for all
// string timestamps stored in the database and returned in API responses.
// All time.Time → string conversions must use this layout.
const TimestampFormat = time.RFC3339
