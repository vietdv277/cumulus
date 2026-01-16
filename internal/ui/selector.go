package ui

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	pkgtypes "github.com/vietdv277/cumulus/pkg/types"
)

// SelectInstance displays an interactive selector for EC2 instances
// and returns the selected instance
func SelectInstance(instances []pkgtypes.Instance) (*pkgtypes.Instance, error) {
	if len(instances) == 0 {
		return nil, fmt.Errorf("no instances available")
	}

	// Custom templates for display
	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}",
		Active:   "▸ {{ .Name | cyan }} ({{ .ID | yellow }}) {{ .PrivateIP | green }} {{ .ASG | faint }}",
		Inactive: "  {{ .Name | cyan }} ({{ .ID | yellow }}) {{ .PrivateIP | green }} {{ .ASG | faint }}",
		Selected: "✔ {{ .Name | green }} ({{ .ID }})",
		Details: `
--------- Instance Details ----------
{{ "Name:" | faint }}	{{ .Name }}
{{ "ID:" | faint }}	{{ .ID }}
{{ "Private IP:" | faint }}	{{ .PrivateIP }}
{{ "Type:" | faint }}	{{ .Type }}
{{ "AZ:" | faint }}	{{ .AZ }}
{{ "ASG:" | faint }}	{{ .ASG }}`,
	}

	// Search function for filtering
	searcher := func(input string, index int) bool {
		instance := instances[index]
		name := strings.ToLower(instance.Name)
		id := strings.ToLower(instance.ID)
		ip := strings.ToLower(instance.PrivateIP)
		asg := strings.ToLower(instance.ASG)
		input = strings.ToLower(input)

		return strings.Contains(name, input) ||
			strings.Contains(id, input) ||
			strings.Contains(ip, input) ||
			strings.Contains(asg, input)
	}

	prompt := promptui.Select{
		Label:     color.CyanString("Select an instance to connect:"),
		Items:     instances,
		Templates: templates,
		Size:      15,
		Searcher:  searcher,
	}

	idx, _, err := prompt.Run()
	if err != nil {
		return nil, fmt.Errorf("selection cancelled: %w", err)
	}

	return &instances[idx], nil
}
