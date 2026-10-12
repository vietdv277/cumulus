package aws

import (
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/vietdv277/cumulus/pkg/types"
)

func TestIsInstanceID(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"i-0abc123def456", true},
		{"i-", true},
		{"i", false},
		{"instance-1", false},
		{"web-server", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := isInstanceID(tt.in); got != tt.want {
				t.Errorf("isInstanceID(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestSelectInstance(t *testing.T) {
	one := vmMatch{ID: "i-111", State: "running", VM: types.VM{ID: "i-111", Name: "web"}}
	two := vmMatch{ID: "i-222", State: "stopped", VM: types.VM{ID: "i-222", Name: "web"}}

	tests := []struct {
		name     string
		matches  []vmMatch
		query    string
		wantID   string
		wantErr  string
		errParts []string
	}{
		{
			name:    "no matches",
			matches: nil,
			query:   "web",
			wantErr: "instance not found: web",
		},
		{
			name:    "single match wins",
			matches: []vmMatch{one},
			query:   "web",
			wantID:  "i-111",
		},
		{
			name:     "ambiguous name is rejected",
			matches:  []vmMatch{one, two},
			query:    "web",
			wantErr:  `multiple instances named "web"`,
			errParts: []string{"i-111 (running)", "i-222 (stopped)", "instance ID"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm, err := selectInstance(tt.matches, tt.query)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("selectInstance() unexpected error: %v", err)
				}
				if vm.ID != tt.wantID {
					t.Errorf("selectInstance() ID = %q, want %q", vm.ID, tt.wantID)
				}
				return
			}
			if err == nil {
				t.Fatalf("selectInstance() error = nil, want %q", tt.wantErr)
			}
			if got := err.Error(); !strings.HasPrefix(got, tt.wantErr) {
				t.Errorf("selectInstance() error = %q, want prefix %q", got, tt.wantErr)
			}
			for _, part := range tt.errParts {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("selectInstance() error = %q, missing %q", err.Error(), part)
				}
			}
		})
	}
}

func TestInstanceStateNameNilState(t *testing.T) {
	// A nil State used to panic on i.State.Name.
	var inst ec2types.Instance
	if got := instanceStateName(inst); got != "" {
		t.Errorf("instanceStateName(nil State) = %q, want empty", got)
	}

	inst.State = &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning}
	if got := instanceStateName(inst); got != ec2types.InstanceStateNameRunning {
		t.Errorf("instanceStateName() = %q, want running", got)
	}
}

func TestEC2ToVMNilState(t *testing.T) {
	// Regression: i.State.Name panicked when State was nil.
	vm := ec2ToVM(ec2types.Instance{InstanceId: aws.String("i-123")})
	if vm.ID != "i-123" {
		t.Errorf("ec2ToVM() ID = %q, want i-123", vm.ID)
	}
	if vm.State != types.VMStateUnknown {
		t.Errorf("ec2ToVM() State = %q, want unknown", vm.State)
	}
}
