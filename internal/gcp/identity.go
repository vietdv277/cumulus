package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/oauth2/google"
)

// CallerIdentity holds the resolved GCP identity from Application Default Credentials.
type CallerIdentity struct {
	// Email is the account email — service account address or user email.
	Email string
	// ProjectID is the GCP project associated with the credentials.
	ProjectID string
	// TokenType is the ADC credential type: "service_account", "authorized_user",
	// "external_account", or "impersonated_service_account".
	TokenType string
}

// GetCallerIdentity verifies GCP Application Default Credentials and returns
// the resolved identity. Mirrors the aws.GetCallerIdentity pattern.
func GetCallerIdentity(project, region string) (*CallerIdentity, error) {
	ctx := context.Background()

	creds, err := google.FindDefaultCredentials(ctx, scopeCloudPlatform)
	if err != nil {
		return nil, fmt.Errorf(
			"no application default credentials found — run 'gcloud auth application-default login': %w",
			err,
		)
	}

	// Obtain a token to confirm the credentials are actually valid and not expired.
	token, err := creds.TokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf(
			"failed to refresh GCP credentials — run 'gcloud auth application-default login': %w",
			err,
		)
	}
	if !token.Valid() {
		return nil, fmt.Errorf(
			"GCP credentials are expired — run 'gcloud auth application-default login'",
		)
	}

	identity := &CallerIdentity{ProjectID: project}

	// Credentials file may carry the project ID directly.
	if creds.ProjectID != "" {
		identity.ProjectID = creds.ProjectID
	}

	// Parse the ADC file for credential type and service-account email.
	adcPath := defaultADCPath()
	if data, readErr := os.ReadFile(adcPath); readErr == nil {
		var adc adcJSON
		if jsonErr := json.Unmarshal(data, &adc); jsonErr == nil {
			identity.TokenType = adc.Type
			if adc.Type == credTypeServiceAccount && adc.ClientEmail != "" {
				identity.Email = adc.ClientEmail
			}
		}
	}

	// For user/external credentials the email is not in the file; fetch it
	// from the Google userinfo endpoint using the access token.
	if identity.Email == "" {
		if email, err := fetchUserEmail(ctx, token.AccessToken); err == nil {
			identity.Email = email
		}
	}

	return identity, nil
}

// Credential type constants, matching the "type" field in ADC JSON files.
const (
	credTypeServiceAccount            = "service_account"
	credTypeAuthorizedUser            = "authorized_user"
	credTypeExternalAccount           = "external_account"
	credTypeImpersonatedServiceAccount = "impersonated_service_account"
)

// adcJSON matches the fields we care about in an ADC credentials file.
type adcJSON struct {
	Type        string `json:"type"`
	ClientEmail string `json:"client_email"` // present for service_account
}

// userinfoResponse is the subset of fields returned by the Google userinfo API.
type userinfoResponse struct {
	Email            string `json:"email"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// defaultADCPath returns the path to the active ADC credentials file.
// Respects the GOOGLE_APPLICATION_CREDENTIALS environment variable.
func defaultADCPath() string {
	if env := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"); env != "" {
		return env
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "gcloud", "application_default_credentials.json")
}

// fetchUserEmail calls the Google OAuth2 userinfo endpoint to retrieve the
// account email associated with the given access token. Used for
// authorized_user and external_account credentials where the email is not
// stored locally.
func fetchUserEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet,
		"https://www.googleapis.com/oauth2/v1/userinfo",
		nil,
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	httpClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var info userinfoResponse
	if err := json.Unmarshal(body, &info); err != nil {
		return "", err
	}
	if info.Error != "" {
		return "", fmt.Errorf("%s: %s", info.Error, info.ErrorDescription)
	}

	return info.Email, nil
}
