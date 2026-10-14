package gcp

import (
	"errors"
	"fmt"
	"net/http"

	"google.golang.org/api/googleapi"

	"github.com/vietdv277/cumulus/pkg/provider"
)

// isNotFound reports whether err is a googleapi.Error with HTTP 404. Only this
// qualifies as "not found"; everything else (permission denied, expired token,
// quota) must surface to the caller.
func isNotFound(err error) bool {
	var apiErr *googleapi.Error
	return errors.As(err, &apiErr) && apiErr.Code == http.StatusNotFound
}

// isTransient reports whether err is a googleapi.Error worth retrying later:
// rate limited (429) or the service briefly unavailable (500, 503).
func isTransient(err error) bool {
	var apiErr *googleapi.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.Code {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable:
		return true
	}
	return false
}

// notFoundErr reports name as missing, wrapping the provider's ErrNotFound
// sentinel so callers can detect it with errors.Is.
func notFoundErr(what, name string) error {
	return fmt.Errorf("%s %q not found: %w", what, name, provider.ErrNotFound)
}
