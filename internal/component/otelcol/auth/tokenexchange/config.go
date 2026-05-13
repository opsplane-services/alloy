package tokenexchange

import (
	"net/http"
	"time"
)

// Config is the internal configuration for the token exchange extension.
// It is built from the component's Arguments.
type Config struct {
	Provider              string
	TokenFile             string
	TokenFilePollInterval time.Duration
	ExpiryBuffer          time.Duration
	Header                string
	Scheme                string

	// GCP-specific fields.
	GCPManagedIdentity         bool
	GCPSTSEndpoint             string
	GCPIAMEndpoint             string
	GCPMetadataEndpoint        string
	GCPMetadataServiceAccount  string
	GCPAudience                string
	GCPScopes                  []string
	GCPSubjectTokenType        string
	GCPRequestedTokenType      string
	GCPServiceAccountEmail     string
	GCPServiceAccountTokenLife time.Duration

	// Azure-specific fields.
	AzureManagedIdentity bool
	AzureTenantID        string
	AzureClientID        string
	AzureAuthorityHost   string
	AzureScopes          []string

	// HTTPClient is used for token exchange HTTP requests.
	HTTPClient *http.Client
}
