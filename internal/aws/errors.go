package aws

import (
	"errors"

	"github.com/aws/smithy-go"
)

// isNotFoundErr reports whether err is an AWS API error whose code matches one
// of the given not-found codes. Only these errors may be reported as
// "not found" — everything else (AccessDenied, expired SSO token, ...) must be
// surfaced to the caller.
func isNotFoundErr(err error, codes ...string) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	code := apiErr.ErrorCode()
	for _, c := range codes {
		if code == c {
			return true
		}
	}
	return false
}
