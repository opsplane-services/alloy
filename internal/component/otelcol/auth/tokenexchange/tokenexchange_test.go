package tokenexchange

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/grafana/alloy/internal/component/otelcol/auth"
	"github.com/grafana/alloy/internal/runtime/componenttest"
	"github.com/grafana/alloy/internal/util"
	"github.com/grafana/alloy/syntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	extauth "go.opentelemetry.io/collector/extension/extensionauth"
)

func TestArguments_Validate_GCP(t *testing.T) {
	args := Arguments{
		Provider:  "gcp",
		TokenFile: "/var/run/secrets/token",
		GCP: &GCPArguments{
			Audience: "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/providers/provider",
		},
	}
	require.NoError(t, args.Validate())
}

func TestArguments_Validate_Azure(t *testing.T) {
	args := Arguments{
		Provider:  "azure",
		TokenFile: "/var/run/secrets/token",
		Azure: &AzureArguments{
			TenantID: "tenant-id",
			ClientID: "client-id",
			Scopes:   []string{"https://monitor.azure.com/.default"},
		},
	}
	require.NoError(t, args.Validate())
}

func TestArguments_Validate_MissingProvider(t *testing.T) {
	args := Arguments{
		TokenFile: "/var/run/secrets/token",
	}
	err := args.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider must be set")
}

func TestArguments_Validate_MissingTokenFile(t *testing.T) {
	args := Arguments{
		Provider: "gcp",
		GCP: &GCPArguments{
			Audience: "test",
		},
	}
	err := args.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token_file must be set")
}

func TestArguments_Validate_ManagedIdentity_NoTokenFileRequired(t *testing.T) {
	// GCP managed identity should not require token_file.
	args := Arguments{
		Provider: "gcp",
		GCP: &GCPArguments{
			ManagedIdentity: true,
			Scopes:          []string{"https://www.googleapis.com/auth/cloud-platform"},
		},
	}
	require.NoError(t, args.Validate())

	// Azure managed identity should not require token_file.
	args = Arguments{
		Provider: "azure",
		Azure: &AzureArguments{
			ManagedIdentity: true,
			Scopes:          []string{"https://monitor.azure.com/.default"},
		},
	}
	require.NoError(t, args.Validate())
}

func TestArguments_Validate_AzureManagedIdentity_NoTenantRequired(t *testing.T) {
	args := Arguments{
		Provider: "azure",
		Azure: &AzureArguments{
			ManagedIdentity: true,
			Scopes:          []string{"https://monitor.azure.com/.default"},
		},
	}
	require.NoError(t, args.Validate())
}

func TestArguments_Validate_AzureManagedIdentity_UserAssigned(t *testing.T) {
	args := Arguments{
		Provider: "azure",
		Azure: &AzureArguments{
			ManagedIdentity: true,
			ClientID:        "user-assigned-client-id",
			Scopes:          []string{"https://monitor.azure.com/.default"},
		},
	}
	require.NoError(t, args.Validate())
}

func TestArguments_Validate_GCPManagedIdentity_NoAudienceRequired(t *testing.T) {
	args := Arguments{
		Provider: "gcp",
		GCP: &GCPArguments{
			ManagedIdentity: true,
		},
	}
	require.NoError(t, args.Validate())
}

func TestArguments_Validate_GCPManagedIdentity_IDToken_RequiresAudience(t *testing.T) {
	args := Arguments{
		Provider: "gcp",
		GCP: &GCPArguments{
			ManagedIdentity:    true,
			RequestedTokenType: "urn:ietf:params:oauth:token-type:id_token",
		},
	}
	err := args.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gcp.audience must be set when requesting ID tokens")
}

func TestArguments_Validate_GCPManagedIdentity_IDToken_WithAudience(t *testing.T) {
	args := Arguments{
		Provider: "gcp",
		GCP: &GCPArguments{
			ManagedIdentity:    true,
			RequestedTokenType: "urn:ietf:params:oauth:token-type:id_token",
			Audience:           "https://my-service.example.com",
		},
	}
	require.NoError(t, args.Validate())
}

func TestArguments_Validate_UnsupportedProvider(t *testing.T) {
	args := Arguments{
		Provider:  "aws",
		TokenFile: "/token",
	}
	err := args.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported provider")
}

func TestArguments_Validate_GCPMissingBlock(t *testing.T) {
	args := Arguments{
		Provider:  "gcp",
		TokenFile: "/token",
	}
	err := args.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gcp block is required")
}

func TestArguments_Validate_GCPWithAzureBlock(t *testing.T) {
	args := Arguments{
		Provider:  "gcp",
		TokenFile: "/token",
		GCP:       &GCPArguments{Audience: "test"},
		Azure:     &AzureArguments{TenantID: "t", ClientID: "c", Scopes: []string{"s"}},
	}
	err := args.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "azure block must not be set")
}

func TestArguments_Validate_AzureMissingFields(t *testing.T) {
	args := Arguments{
		Provider:  "azure",
		TokenFile: "/token",
		Azure:     &AzureArguments{Scopes: []string{"s"}},
	}
	err := args.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "azure.tenant_id must be set")
}

func TestArguments_Validate_AzureMissingScopes(t *testing.T) {
	args := Arguments{
		Provider:  "azure",
		TokenFile: "/token",
		Azure:     &AzureArguments{TenantID: "t", ClientID: "c"},
	}
	err := args.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "azure.scopes must be set")
}

func TestArguments_Validate_GCPMissingAudience(t *testing.T) {
	args := Arguments{
		Provider:  "gcp",
		TokenFile: "/token",
		GCP:       &GCPArguments{ManagedIdentity: false},
	}
	err := args.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gcp.audience must be set")
}

func TestArguments_SetToDefault(t *testing.T) {
	var args Arguments
	args.SetToDefault()

	assert.Equal(t, 5*time.Minute, args.TokenFilePollInterval)
	assert.Equal(t, time.Minute, args.ExpiryBuffer)
}

func TestArguments_BuildConfig_GCP(t *testing.T) {
	args := Arguments{
		Provider:  "gcp",
		TokenFile: "/token",
		GCP: &GCPArguments{
			Audience:            "test-audience",
			ServiceAccountEmail: "sa@project.iam.gserviceaccount.com",
		},
	}
	args.SetToDefault()
	cfg := args.buildConfig()

	assert.Equal(t, "gcp", cfg.Provider)
	assert.Equal(t, "/token", cfg.TokenFile)
	assert.Equal(t, "https://sts.googleapis.com/v1/token", cfg.GCPSTSEndpoint)
	assert.Equal(t, "test-audience", cfg.GCPAudience)
	assert.Equal(t, []string{"https://www.googleapis.com/auth/cloud-platform"}, cfg.GCPScopes)
	assert.Equal(t, "urn:ietf:params:oauth:token-type:jwt", cfg.GCPSubjectTokenType)
	assert.Equal(t, "sa@project.iam.gserviceaccount.com", cfg.GCPServiceAccountEmail)
	assert.Equal(t, time.Hour, cfg.GCPServiceAccountTokenLife)
}

func TestUnmarshalAlloy_GCP(t *testing.T) {
	cfg := `
		provider   = "gcp"
		token_file = "/var/run/secrets/tokens/oidc-token"

		gcp {
			audience = "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/providers/prov"
		}
	`

	var args Arguments
	require.NoError(t, syntax.Unmarshal([]byte(cfg), &args))
	assert.Equal(t, "gcp", args.Provider)
	assert.Equal(t, "/var/run/secrets/tokens/oidc-token", args.TokenFile)
	assert.NotNil(t, args.GCP)
	assert.Contains(t, args.GCP.Audience, "workloadIdentityPools")
}

func TestUnmarshalAlloy_Azure(t *testing.T) {
	cfg := `
		provider   = "azure"
		token_file = "/var/run/secrets/azure/tokens/azure-identity-token"

		azure {
			tenant_id = "my-tenant"
			client_id = "my-client"
			scopes    = ["https://monitor.azure.com/.default"]
		}
	`

	var args Arguments
	require.NoError(t, syntax.Unmarshal([]byte(cfg), &args))
	assert.Equal(t, "azure", args.Provider)
	assert.NotNil(t, args.Azure)
	assert.Equal(t, "my-tenant", args.Azure.TenantID)
	assert.Equal(t, "my-client", args.Azure.ClientID)
	assert.Equal(t, []string{"https://monitor.azure.com/.default"}, args.Azure.Scopes)
}

func TestComponent_GCP_Integration(t *testing.T) {
	// Set up a mock STS server.
	stsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(gcpSTSResponse{
			AccessToken: "test-gcp-token",
			ExpiresIn:   3600,
			TokenType:   "Bearer",
		})
	}))
	defer stsServer.Close()

	// Write a temporary token file.
	tokenDir := t.TempDir()
	tokenFile := filepath.Join(tokenDir, "token")
	require.NoError(t, os.WriteFile(tokenFile, []byte("k8s-sa-token"), 0600))

	ctx := componenttest.TestContext(t)
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	l := util.TestLogger(t)
	ctrl, err := componenttest.NewControllerFromID(l, "otelcol.auth.tokenexchange")
	require.NoError(t, err)

	var args Arguments
	args.SetToDefault()
	args.Provider = "gcp"
	args.TokenFile = tokenFile
	args.GCP = &GCPArguments{
		Audience:         "test-audience",
		STSEndpoint:      stsServer.URL,
		SubjectTokenType: "urn:ietf:params:oauth:token-type:jwt",
		Scopes:           []string{"https://www.googleapis.com/auth/cloud-platform"},
	}

	go func() {
		err := ctrl.Run(ctx, args)
		require.NoError(t, err)
	}()

	require.NoError(t, ctrl.WaitRunning(0), "component never started")
	require.NoError(t, ctrl.WaitExports(0), "component never exported anything")

	// Get the auth extension and verify it injects the token.
	exports := ctrl.Exports().(auth.Exports)
	require.NotNil(t, exports.Handler)

	clientExt, err := exports.Handler.GetExtension(auth.Client)
	require.NoError(t, err)

	clientAuth, ok := clientExt.Extension.(extauth.HTTPClient)
	require.True(t, ok, "extension does not implement extensionauth.HTTPClient")

	rt, err := clientAuth.RoundTripper(http.DefaultTransport)
	require.NoError(t, err)

	// Make a request through our round tripper to a test server that checks the header.
	testSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-gcp-token", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	defer testSrv.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, testSrv.URL, nil)
	require.NoError(t, err)

	resp, err := (&http.Client{Transport: rt}).Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
