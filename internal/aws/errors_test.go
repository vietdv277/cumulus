package aws

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aws/smithy-go"
)

// fakeAPIError is a stand-in for the SDK's typed API errors.
type fakeAPIError struct {
	code string
	msg  string
}

func (e fakeAPIError) Error() string        { return e.msg }
func (e fakeAPIError) ErrorCode() string    { return e.code }
func (e fakeAPIError) ErrorMessage() string { return e.msg }
func (e fakeAPIError) ErrorFault() smithy.ErrorFault {
	return smithy.FaultClient
}

func TestIsNotFoundErr(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		codes []string
		want  bool
	}{
		{
			name:  "matching code",
			err:   fakeAPIError{code: "InvalidInstanceID.NotFound", msg: "instance not found"},
			codes: []string{"InvalidInstanceID.NotFound"},
			want:  true,
		},
		{
			name:  "access denied is not not-found",
			err:   fakeAPIError{code: "AccessDenied", msg: "denied"},
			codes: []string{"InvalidInstanceID.NotFound"},
			want:  false,
		},
		{
			name:  "wrapped api error matches",
			err:   fmt.Errorf("describe failed: %w", fakeAPIError{code: "DBInstanceNotFound", msg: "no db"}),
			codes: []string{"DBInstanceNotFound"},
			want:  true,
		},
		{
			name:  "wrapped api error with different code",
			err:   fmt.Errorf("describe failed: %w", fakeAPIError{code: "Throttling", msg: "slow down"}),
			codes: []string{"DBInstanceNotFound"},
			want:  false,
		},
		{
			name:  "plain error is not not-found",
			err:   errors.New("connection refused"),
			codes: []string{"InvalidInstanceID.NotFound"},
			want:  false,
		},
		{
			name:  "one of several codes",
			err:   fakeAPIError{code: "ResourceNotFoundException", msg: "missing"},
			codes: []string{"DBInstanceNotFound", "ResourceNotFoundException"},
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isNotFoundErr(tt.err, tt.codes...); got != tt.want {
				t.Errorf("isNotFoundErr() = %v, want %v", got, tt.want)
			}
		})
	}
}
