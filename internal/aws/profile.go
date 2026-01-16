package aws

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	pkgtypes "github.com/vietdv277/cumulus/pkg/types"
)

// profileSection matches [profile-name] or [profile profile-name]
var (
	credentialsSectionRe = regexp.MustCompile(`^\[([^\]]+)\]$`)
	configSectionRe      = regexp.MustCompile(`^\[profile\s+([^\]]+)\]$`)
	configDefaultRe      = regexp.MustCompile(`^\[default\]$`)
	regionRe             = regexp.MustCompile(`^\s*region\s*=\s*(.+)$`)
)

// ListProfiles reads AWS profiles from ~/.aws/credentials and ~/.aws/config
func ListProfiles() ([]pkgtypes.AWSProfile, error) {
	profileMap := make(map[string]*pkgtypes.AWSProfile)

	// Parse credentials file
	credProfiles, err := parseCredentialsFile()
	if err == nil {
		for _, p := range credProfiles {
			profileMap[p.Name] = &p
		}
	}

	// Parse config file (may add region info or new profiles)
	configProfiles, err := parseConfigFile()
	if err == nil {
		for _, p := range configProfiles {
			if existing, ok := profileMap[p.Name]; ok {
				// Merge: add region if not set
				if existing.Region == "" && p.Region != "" {
					existing.Region = p.Region
				}
			} else {
				// New profile from config (SSO profiles, etc.)
				profileMap[p.Name] = &p
			}
		}
	}

	// Convert to sorted slice
	var profiles []pkgtypes.AWSProfile
	for _, p := range profileMap {
		profiles = append(profiles, *p)
	}

	sort.Slice(profiles, func(i, j int) bool {
		// Put "default" first, then sort alphabetically
		if profiles[i].Name == "default" {
			return true
		}
		if profiles[j].Name == "default" {
			return false
		}
		return profiles[i].Name < profiles[j].Name
	})

	return profiles, nil
}

// ValidateProfile checks if a profile exists
func ValidateProfile(name string) bool {
	profiles, err := ListProfiles()
	if err != nil {
		return false
	}

	for _, p := range profiles {
		if p.Name == name {
			return true
		}
	}
	return false
}

// parseCredentialsFile parses ~/.aws/credentials
func parseCredentialsFile() ([]pkgtypes.AWSProfile, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	credPath := filepath.Join(home, ".aws", "credentials")
	return parseINIFile(credPath, "credentials", false)
}

// parseConfigFile parses ~/.aws/config
func parseConfigFile() ([]pkgtypes.AWSProfile, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	configPath := filepath.Join(home, ".aws", "config")
	return parseINIFile(configPath, "config", true)
}

// parseINIFile parses an AWS INI-style config file
func parseINIFile(path, source string, isConfigFile bool) ([]pkgtypes.AWSProfile, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var profiles []pkgtypes.AWSProfile
	var currentProfile *pkgtypes.AWSProfile

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		// Check for section header
		if isConfigFile {
			// Config file: [profile name] or [default]
			if configDefaultRe.MatchString(line) {
				if currentProfile != nil {
					profiles = append(profiles, *currentProfile)
				}
				currentProfile = &pkgtypes.AWSProfile{
					Name:   "default",
					Source: source,
				}
				continue
			}

			if matches := configSectionRe.FindStringSubmatch(line); len(matches) == 2 {
				if currentProfile != nil {
					profiles = append(profiles, *currentProfile)
				}
				currentProfile = &pkgtypes.AWSProfile{
					Name:   strings.TrimSpace(matches[1]),
					Source: source,
				}
				continue
			}
		} else {
			// Credentials file: [profile-name]
			if matches := credentialsSectionRe.FindStringSubmatch(line); len(matches) == 2 {
				if currentProfile != nil {
					profiles = append(profiles, *currentProfile)
				}
				currentProfile = &pkgtypes.AWSProfile{
					Name:   strings.TrimSpace(matches[1]),
					Source: source,
				}
				continue
			}
		}

		// Check for region setting
		if currentProfile != nil {
			if matches := regionRe.FindStringSubmatch(line); len(matches) == 2 {
				currentProfile.Region = strings.TrimSpace(matches[1])
			}
		}
	}

	// Don't forget the last profile
	if currentProfile != nil {
		profiles = append(profiles, *currentProfile)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return profiles, nil
}
