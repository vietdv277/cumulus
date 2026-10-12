package types

import "time"

// AutoScalingGroup represents an AWS Auto Scaling Group
type AutoScalingGroup struct {
	Name            string     `json:"name" yaml:"name"`
	ARN             string     `json:"arn" yaml:"arn"`
	LaunchTemplate  string     `json:"launch_template" yaml:"launch_template"`
	DesiredCapacity int        `json:"desired_capacity" yaml:"desired_capacity"`
	MinSize         int        `json:"min_size" yaml:"min_size"`
	MaxSize         int        `json:"max_size" yaml:"max_size"`
	InstanceCount   int        `json:"instance_count" yaml:"instance_count"` // current running instances
	HealthyCount    int        `json:"healthy_count" yaml:"healthy_count"`
	UnhealthyCount  int        `json:"unhealthy_count" yaml:"unhealthy_count"`
	Status          string     `json:"status" yaml:"status"` // InService, Updating, etc.
	CreatedTime     time.Time  `json:"created_time" yaml:"created_time"`
	AZs             []string   `json:"azs" yaml:"azs"`
	Instances       []Instance `json:"instances,omitempty" yaml:"instances,omitempty"` // for describe command
}
