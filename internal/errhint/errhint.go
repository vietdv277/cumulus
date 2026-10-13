// Package errhint classifies AWS and GCP credential and permission errors
// into short recovery hints for the CLI's error output.
package errhint

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/credentials/ssocreds"
	"github.com/aws/smithy-go"
	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"
)

// Env names the context an error happened in; any field may be empty.
type Env struct {
	Provider string // "aws", "gcp" or "" (legacy commands)
	Context  string // resolved context name, e.g. "aws:prod"
	Profile  string // AWS profile
	Project  string // GCP project
}

// Hints returns recovery hints for err, most specific first, or nil when none
// apply. Classification is by error type and API code first, message text
// only as a fallback. At most one class applies, checked in this order:
// expired AWS credentials, expired GCP credentials, AWS permission denied,
// GCP permission denied or API disabled. Anything unlisted returns nil — a
// wrong hint is worse than none.
func Hints(err error, env Env) []string {
	if err == nil {
		return nil
	}
	if hints := awsCredentialHints(err, env); len(hints) > 0 {
		return hints
	}
	if hints := gcpCredentialHints(err, env); len(hints) > 0 {
		return hints
	}
	if hints := awsPermissionHints(err, env); len(hints) > 0 {
		return hints
	}
	return gcpPermissionHints(err, env)
}

// awsExpiredTokenCodes are API error codes that mean the caller's credentials
// are expired or missing.
var awsExpiredTokenCodes = map[string]bool{
	"ExpiredToken":                true,
	"ExpiredTokenException":       true,
	"InvalidClientTokenId":        true,
	"UnrecognizedClientException": true,
}

// awsCredentialMarkers are lower-cased message fragments that mean expired or
// missing AWS credentials.
var awsCredentialMarkers = []string{
	"sso session has expired",
	"token has expired",
	"refresh cached sso token failed",
	"failed to refresh cached credentials",
	"failed to retrieve credentials",
	"no valid credential sources",
	"security token included in the request is expired",
}

// awsCredentialHints classifies expired or missing AWS credentials.
func awsCredentialHints(err error, env Env) []string {
	var token *ssocreds.InvalidTokenError
	if errors.As(err, &token) {
		return []string{awsLoginHint(env.Profile)}
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) && awsExpiredTokenCodes[apiErr.ErrorCode()] {
		return []string{awsLoginHint(env.Profile)}
	}
	if containsAny(strings.ToLower(err.Error()), awsCredentialMarkers) {
		return []string{awsLoginHint(env.Profile)}
	}
	return nil
}

// awsLoginHint tells the user how to refresh AWS SSO credentials.
func awsLoginHint(profile string) string {
	if profile == "" {
		return "AWS credentials are expired or missing — run 'aws sso login' (or set AWS_PROFILE)"
	}
	return fmt.Sprintf("AWS credentials for profile %q are expired or missing — run 'aws sso login --profile %s'", profile, profile)
}

// gcpLoginHint tells the user how to refresh Application Default Credentials.
const gcpLoginHint = "GCP Application Default Credentials are expired or missing — run 'gcloud auth application-default login'"

// gcpCredentialHints classifies expired or missing GCP credentials.
func gcpCredentialHints(err error, _ Env) []string {
	var retrieve *oauth2.RetrieveError
	if errors.As(err, &retrieve) {
		return []string{gcpLoginHint}
	}
	var gErr *googleapi.Error
	if errors.As(err, &gErr) && gErr.Code == 401 {
		return []string{gcpLoginHint}
	}
	if strings.Contains(strings.ToLower(err.Error()), "could not find default credentials") {
		return []string{gcpLoginHint}
	}
	return nil
}

// awsDeniedCodes are API error codes that mean IAM denied the call.
var awsDeniedCodes = map[string]bool{
	"AccessDenied":                true,
	"AccessDeniedException":       true,
	"UnauthorizedOperation":       true,
	"UnauthorizedAccess":          true,
	"AuthorizationError":          true,
	"AuthorizationErrorException": true,
	"Forbidden":                   true,
}

// iamActionRe extracts the denied IAM action from an AWS denial message
// ("... is not authorized to perform: ec2:DescribeInstances ...").
var iamActionRe = regexp.MustCompile(`not authorized to perform: ([a-z0-9-]+:[A-Za-z0-9*]+)`)

// encodedFailureRe extracts the encoded authorization failure message that
// AWS appends to some denials.
var encodedFailureRe = regexp.MustCompile(`Encoded authorization failure message: ([A-Za-z0-9+/=_%-]+)`)

// awsPermissionHints classifies IAM denials.
func awsPermissionHints(err error, env Env) []string {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) || !awsDeniedCodes[apiErr.ErrorCode()] {
		return nil
	}
	hints := []string{awsDeniedHint(env.Profile, awsAction(err))}
	if strings.Contains(strings.ToLower(err.Error()), "encoded authorization failure message") {
		hints = append(hints, decodeHint(err))
	}
	return hints
}

// awsAction names the denied operation: the IAM action from the message when
// it carries one, else the SDK's service and operation names. Nothing is
// invented that the message does not carry.
func awsAction(err error) string {
	if m := iamActionRe.FindStringSubmatch(err.Error()); m != nil {
		return m[1]
	}
	var opErr *smithy.OperationError
	if errors.As(err, &opErr) {
		return opErr.ServiceID + " " + opErr.OperationName
	}
	return "this operation"
}

// awsDeniedHint names the profile that was denied.
func awsDeniedHint(profile, action string) string {
	if profile == "" {
		return fmt.Sprintf("the current AWS credentials are not allowed to call %s — check its IAM policy or use another context", action)
	}
	return fmt.Sprintf("profile %q is not allowed to call %s — check its IAM policy or use another context", profile, action)
}

// decodeHint points at the encoded authorization failure message so it can be
// decoded; the blob is quoted when the message carries one.
func decodeHint(err error) string {
	if m := encodedFailureRe.FindStringSubmatch(err.Error()); m != nil {
		return fmt.Sprintf("decode the details with 'aws sts decode-authorization-message --encoded-message %s'", m[1])
	}
	return "decode the details with 'aws sts decode-authorization-message --encoded-message <message>'"
}

// gcpPermissionHints classifies GCP 403s: a disabled API or a missing
// permission.
func gcpPermissionHints(err error, env Env) []string {
	var gErr *googleapi.Error
	if !errors.As(err, &gErr) || gErr.Code != 403 {
		return nil
	}
	info := errorInfo(gErr)
	if gcpDisabled(gErr, info) {
		return []string{gcpDisabledHint(info, env.Project)}
	}
	return []string{gcpPermissionHint(err, info, env.Project)}
}

// errorInfo returns the first google.rpc.ErrorInfo detail of a googleapi
// Error, or nil when the response carries none.
func errorInfo(gErr *googleapi.Error) map[string]any {
	for _, d := range gErr.Details {
		m, ok := d.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := m["@type"].(string); strings.HasSuffix(t, "google.rpc.ErrorInfo") {
			return m
		}
	}
	return nil
}

// gcpDisabled reports whether a 403 is about a disabled API: the reason
// appears in the ErrorInfo or in the top-level error items.
func gcpDisabled(gErr *googleapi.Error, info map[string]any) bool {
	if reason, _ := info["reason"].(string); reason == "SERVICE_DISABLED" || reason == "accessNotConfigured" {
		return true
	}
	for _, item := range gErr.Errors {
		if item.Reason == "SERVICE_DISABLED" || item.Reason == "accessNotConfigured" {
			return true
		}
	}
	return false
}

// gcpDisabledHint tells the user to enable the disabled API; the service name
// comes from the ErrorInfo metadata and the hint stays generic without it.
func gcpDisabledHint(info map[string]any, project string) string {
	metadata, _ := info["metadata"].(map[string]any)
	service, _ := metadata["service"].(string)

	subject := "the required API"
	enable := ""
	if service != "" {
		subject = fmt.Sprintf("the %s API", service)
		enable = fmt.Sprintf(" — run 'gcloud services enable %s", service)
		if project != "" {
			enable += " --project " + project
		}
		enable += "'"
	}
	where := ""
	if project != "" {
		where = fmt.Sprintf(" for project %q", project)
	}
	return subject + " is disabled" + where + enable
}

// gcpPermissionRe extracts a denied GCP permission from the message
// ("Permission 'compute.instances.get' denied ...").
var gcpPermissionRe = regexp.MustCompile(`[Pp]ermission ['"]?([a-z][a-zA-Z0-9]*(?:\.[a-zA-Z0-9]+){2,})['"]?`)

// gcpPermissionHint names the missing GCP permission: from the ErrorInfo
// metadata, else the message, else a generic one.
func gcpPermissionHint(err error, info map[string]any, project string) string {
	metadata, _ := info["metadata"].(map[string]any)
	permission, _ := metadata["permission"].(string)
	if permission == "" {
		if m := gcpPermissionRe.FindStringSubmatch(err.Error()); m != nil {
			permission = m[1]
		}
	}
	if permission == "" {
		permission = "this operation"
	}
	onProject := ""
	if project != "" {
		onProject = fmt.Sprintf(" on project %q", project)
	}
	return fmt.Sprintf("the GCP credentials lack %s%s — grant it or use another context", permission, onProject)
}

// containsAny reports whether s contains any of the markers.
func containsAny(s string, markers []string) bool {
	for _, m := range markers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}
