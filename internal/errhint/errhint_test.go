package errhint

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/aws/aws-sdk-go-v2/credentials/ssocreds"
	"github.com/aws/smithy-go"
	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"
)

var (
	awsEnv = Env{Provider: "aws", Context: "aws:prod", Profile: "dev"}
	gcpEnv = Env{Provider: "gcp", Context: "gcp:staging", Project: "example-project"}
)

// TestHints covers every class and branch with synthetic values wrapped the
// way the SDKs wrap them (OperationError / fmt.Errorf %w chains).
func TestHints(t *testing.T) {
	tests := []struct {
		name string
		err  error
		env  Env
		want []string
	}{
		// 1. AWS credentials expired/missing.
		{
			name: "expired sso token type",
			err: fmt.Errorf("list instances: %w", &smithy.OperationError{
				ServiceID: "EC2", OperationName: "DescribeInstances",
				Err: &ssocreds.InvalidTokenError{Err: errors.New("sso session has expired")},
			}),
			env:  awsEnv,
			want: []string{`AWS credentials for profile "dev" are expired or missing — run 'aws sso login --profile dev'`},
		},
		{
			name: "expired token api code",
			err: fmt.Errorf("whoami: %w", &smithy.OperationError{
				ServiceID: "STS", OperationName: "GetCallerIdentity",
				Err: &smithy.GenericAPIError{Code: "ExpiredToken", Message: "expired token"},
			}),
			env:  awsEnv,
			want: []string{`AWS credentials for profile "dev" are expired or missing — run 'aws sso login --profile dev'`},
		},
		{
			name: "unrecognized client api code",
			err: fmt.Errorf("list objects: %w", &smithy.OperationError{
				ServiceID: "S3", OperationName: "ListObjectsV2",
				Err: &smithy.GenericAPIError{Code: "UnrecognizedClientException", Message: "The security token included in the request is invalid"},
			}),
			env:  awsEnv,
			want: []string{`AWS credentials for profile "dev" are expired or missing — run 'aws sso login --profile dev'`},
		},
		{
			name: "credential message marker",
			err:  fmt.Errorf("list instances: %w", errors.New("operation error EC2: DescribeInstances, failed to retrieve credentials, no valid credential sources found")),
			env:  awsEnv,
			want: []string{`AWS credentials for profile "dev" are expired or missing — run 'aws sso login --profile dev'`},
		},
		{
			name: "sso session message marker",
			err:  fmt.Errorf("list instances: %w", errors.New("operation error EC2: DescribeInstances, failed to refresh cached credentials, sso session has expired")),
			env:  awsEnv,
			want: []string{`AWS credentials for profile "dev" are expired or missing — run 'aws sso login --profile dev'`},
		},
		{
			name: "expired security token message marker",
			err:  errors.New("get caller identity: the security token included in the request is expired"),
			env:  awsEnv,
			want: []string{`AWS credentials for profile "dev" are expired or missing — run 'aws sso login --profile dev'`},
		},
		{
			name: "empty profile",
			err: fmt.Errorf("list instances: %w", &smithy.OperationError{
				ServiceID: "EC2", OperationName: "DescribeInstances",
				Err: &ssocreds.InvalidTokenError{},
			}),
			env:  Env{Provider: "aws"},
			want: []string{`AWS credentials are expired or missing — run 'aws sso login' (or set AWS_PROFILE)`},
		},

		// 2. GCP credentials expired/missing.
		{
			name: "oauth retrieve error",
			err: fmt.Errorf("failed to refresh GCP credentials: %w", &oauth2.RetrieveError{
				ErrorCode: "invalid_grant", ErrorDescription: "Token has been expired or revoked.",
			}),
			env:  gcpEnv,
			want: []string{`GCP Application Default Credentials are expired or missing — run 'gcloud auth application-default login'`},
		},
		{
			name: "googleapi 401",
			err: fmt.Errorf("list vms: %w", &googleapi.Error{
				Code: 401, Message: "Request had invalid authentication credentials.",
				Errors: []googleapi.ErrorItem{{Reason: "authError"}},
			}),
			env:  gcpEnv,
			want: []string{`GCP Application Default Credentials are expired or missing — run 'gcloud auth application-default login'`},
		},
		{
			name: "could not find default credentials message",
			err: fmt.Errorf("no application default credentials found — run 'gcloud auth application-default login': %w",
				errors.New("google: could not find default credentials")),
			env:  gcpEnv,
			want: []string{`GCP Application Default Credentials are expired or missing — run 'gcloud auth application-default login'`},
		},

		// 3. AWS permission denied.
		{
			name: "action from message",
			err: fmt.Errorf("list instances: %w", &smithy.OperationError{
				ServiceID: "EC2", OperationName: "DescribeInstances",
				Err: &smithy.GenericAPIError{
					Code: "UnauthorizedOperation",
					Message: "You are not authorized to perform this operation. User: arn:aws:iam::123456789012:user/dev " +
						"is not authorized to perform: ec2:DescribeInstances on the resource: arn:aws:ec2:us-east-1:123456789012:instance/i-abc123",
				},
			}),
			env:  awsEnv,
			want: []string{`profile "dev" is not allowed to call ec2:DescribeInstances — check its IAM policy or use another context`},
		},
		{
			name: "action from message with empty profile",
			err: fmt.Errorf("list instances: %w", &smithy.OperationError{
				ServiceID: "EC2", OperationName: "DescribeInstances",
				Err: &smithy.GenericAPIError{
					Code: "UnauthorizedOperation",
					Message: "You are not authorized to perform this operation. User: arn:aws:iam::123456789012:user/dev " +
						"is not authorized to perform: ec2:DescribeInstances on the resource: arn:aws:ec2:us-east-1:123456789012:instance/i-abc123",
				},
			}),
			env:  Env{Provider: "aws"},
			want: []string{`the current AWS credentials are not allowed to call ec2:DescribeInstances — check its IAM policy or use another context`},
		},
		{
			name: "action falls back to operation without message action",
			err: fmt.Errorf("list objects: %w", &smithy.OperationError{
				ServiceID: "S3", OperationName: "ListObjectsV2",
				Err: &smithy.GenericAPIError{Code: "AccessDenied", Message: "Access Denied"},
			}),
			env:  awsEnv,
			want: []string{`profile "dev" is not allowed to call S3 ListObjectsV2 — check its IAM policy or use another context`},
		},
		{
			name: "action falls back to this operation without operation error",
			err:  fmt.Errorf("get object: %w", &smithy.GenericAPIError{Code: "Forbidden", Message: "Forbidden"}),
			env:  awsEnv,
			want: []string{`profile "dev" is not allowed to call this operation — check its IAM policy or use another context`},
		},
		{
			name: "encoded authorization failure message adds decode hint",
			err: fmt.Errorf("start instance: %w", &smithy.OperationError{
				ServiceID: "EC2", OperationName: "StartInstances",
				Err: &smithy.GenericAPIError{
					Code: "UnauthorizedOperation",
					Message: "You are not authorized to perform this operation. User: arn:aws:iam::123456789012:user/dev " +
						"is not authorized to perform: ec2:StartInstances on the resource: arn:aws:ec2:us-east-1:123456789012:instance/i-abc123 " +
						"with an explicit deny in a resource-based policy (Encoded authorization failure message: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY)",
				},
			}),
			env: awsEnv,
			want: []string{
				`profile "dev" is not allowed to call ec2:StartInstances — check its IAM policy or use another context`,
				`decode the details with 'aws sts decode-authorization-message --encoded-message wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY'`,
			},
		},

		// 4. GCP permission denied / API disabled.
		{
			name: "disabled api with service from error info",
			err: fmt.Errorf("list vms: %w", &googleapi.Error{
				Code:    403,
				Message: "Compute Engine API has not been used in project 123456789012 before or it is disabled. Enable it by visiting the API console.",
				Details: []any{map[string]any{
					"@type":    "type.googleapis.com/google.rpc.ErrorInfo",
					"reason":   "SERVICE_DISABLED",
					"domain":   "googleapis.com",
					"metadata": map[string]any{"service": "compute.googleapis.com", "consumer": "projects/123456789012"},
				}},
			}),
			env: gcpEnv,
			want: []string{
				`the compute.googleapis.com API is disabled for project "example-project" — run 'gcloud services enable compute.googleapis.com --project example-project'`,
			},
		},
		{
			name: "disabled api without error info falls back to required api",
			err: fmt.Errorf("list databases: %w", &googleapi.Error{
				Code:    403,
				Message: "Access Not Configured. Cloud SQL Admin API has not been used in project 123456789012 before or it is disabled.",
				Errors:  []googleapi.ErrorItem{{Reason: "accessNotConfigured"}},
			}),
			env:  gcpEnv,
			want: []string{`the required API is disabled for project "example-project"`},
		},
		{
			name: "disabled api with service and empty project",
			err: fmt.Errorf("list vms: %w", &googleapi.Error{
				Code: 403,
				Details: []any{map[string]any{
					"@type":    "type.googleapis.com/google.rpc.ErrorInfo",
					"reason":   "SERVICE_DISABLED",
					"metadata": map[string]any{"service": "compute.googleapis.com"},
				}},
			}),
			env:  Env{Provider: "gcp"},
			want: []string{`the compute.googleapis.com API is disabled — run 'gcloud services enable compute.googleapis.com'`},
		},
		{
			name: "permission from error info metadata",
			err: fmt.Errorf("get vm: %w", &googleapi.Error{
				Code:    403,
				Message: "Permission denied on resource project example-project.",
				Details: []any{map[string]any{
					"@type":    "type.googleapis.com/google.rpc.ErrorInfo",
					"reason":   "IAM_PERMISSION_DENIED",
					"metadata": map[string]any{"permission": "compute.instances.get"},
				}},
			}),
			env:  gcpEnv,
			want: []string{`the GCP credentials lack compute.instances.get on project "example-project" — grant it or use another context`},
		},
		{
			name: "permission from message",
			err: fmt.Errorf("get vm: %w", &googleapi.Error{
				Code:    403,
				Message: "Permission 'compute.instances.get' denied on resource: 'projects/example-project/zones/us-central1-a/instances/vm-1'",
			}),
			env:  gcpEnv,
			want: []string{`the GCP credentials lack compute.instances.get on project "example-project" — grant it or use another context`},
		},
		{
			name: "permission falls back to this operation",
			err: fmt.Errorf("get vm: %w", &googleapi.Error{
				Code:    403,
				Message: "The caller does not have permission",
			}),
			env:  gcpEnv,
			want: []string{`the GCP credentials lack this operation on project "example-project" — grant it or use another context`},
		},
		{
			name: "permission with empty project drops the project",
			err: fmt.Errorf("get vm: %w", &googleapi.Error{
				Code:    403,
				Message: "Permission 'compute.instances.get' denied on resource: 'projects/example-project/zones/us-central1-a/instances/vm-1'",
			}),
			env:  Env{Provider: "gcp"},
			want: []string{`the GCP credentials lack compute.instances.get — grant it or use another context`},
		},

		// None of the classes apply.
		{
			name: "plain error",
			err:  errors.New("boom"),
			env:  awsEnv,
			want: nil,
		},
		{
			name: "aws not found code",
			err: fmt.Errorf("get vm: %w", &smithy.OperationError{
				ServiceID: "EC2", OperationName: "DescribeInstances",
				Err: &smithy.GenericAPIError{Code: "InvalidInstanceID.NotFound", Message: "The instance ID 'i-abc123' does not exist"},
			}),
			env:  awsEnv,
			want: nil,
		},
		{
			name: "gcp 404",
			err: fmt.Errorf("get vm: %w", &googleapi.Error{
				Code: 404, Message: "Not Found",
				Errors: []googleapi.ErrorItem{{Reason: "notFound"}},
			}),
			env:  gcpEnv,
			want: nil,
		},
		{
			name: "nil error",
			err:  nil,
			env:  awsEnv,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Hints(tt.err, tt.env); !slices.Equal(got, tt.want) {
				t.Fatalf("Hints() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}
