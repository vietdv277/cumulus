package types

import "time"

// AutoScalingGroup represents an AWS Auto Scaling Group
type AutoScalingGroup struct {
	Name            string
	ARN             string
	LaunchTemplate  string
	DesiredCapacity int
	MinSize         int
	MaxSize         int
	InstanceCount   int // current running instances
	HealthyCount    int
	UnhealthyCount  int
	Status          string // InService, Updating, etc.
	CreatedTime     time.Time
	AZs             []string
	Instances       []Instance // for describe command
}
