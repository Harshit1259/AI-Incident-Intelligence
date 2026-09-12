package api

import "testing"

func TestStatusToErrCode(t *testing.T) {
	tests := map[int]string{
		400: ErrCodeBadRequest,
		401: ErrCodeTokenInvalid,
		413: ErrCodePayloadTooLarge, // was INTERNAL_ERROR
		415: ErrCodeUnsupportedMedia,
		503: ErrCodeServiceUnavailable,
		500: ErrCodeInternal,
	}
	for status, want := range tests {
		if got := statusToErrCode(status); got != want {
			t.Errorf("statusToErrCode(%d) = %q, want %q", status, got, want)
		}
	}
}
