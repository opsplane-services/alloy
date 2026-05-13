// Package tokenexchange provides an otelcol.auth.tokenexchange component that
// exchanges workload identity or managed identity tokens for cloud provider
// access tokens. Supports GCP and Azure.
package tokenexchange

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/grafana/alloy/internal/component"
	"github.com/grafana/alloy/internal/component/otelcol/auth"
	otelcolCfg "github.com/grafana/alloy/internal/component/otelcol/config"
	"github.com/grafana/alloy/internal/featuregate"
	otelcomponent "go.opentelemetry.io/collector/component"
	otelextension "go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/pipeline"
)

func init() {
	component.Register(component.Registration{
		Name:      "otelcol.auth.tokenexchange",
		Stability: featuregate.StabilityExperimental,
		Args:      Arguments{},
		Exports:   auth.Exports{},

		Build: func(opts component.Options, args component.Arguments) (component.Component, error) {
			a := args.(Arguments)
			cfg := a.buildConfig()

			factory := otelextension.NewFactory(
				otelcomponent.MustNewType("tokenexchange"),
				func() otelcomponent.Config { return &extensionConfig{} },
				func(
					ctx context.Context,
					settings otelextension.Settings,
					_ otelcomponent.Config,
				) (otelextension.Extension, error) {
					return newTokenExchangeExtension(ctx, settings, cfg)
				},
				otelcomponent.StabilityLevelAlpha,
			)

			return auth.New(opts, factory, a)
		},
	})
}

// extensionConfig is a placeholder config required by the OTel extension factory.
// The actual configuration is captured in the factory closure.
type extensionConfig struct{}

// Arguments configures the otelcol.auth.tokenexchange component.
type Arguments struct {
	// Provider selects the token exchange backend: "gcp" or "azure".
	Provider string `alloy:"provider,attr"`

	// TokenFile is the path to the source token file (e.g., Kubernetes projected SA token).
	// Required for workload identity. Not used when managed_identity is true.
	TokenFile string `alloy:"token_file,attr,optional"`

	// TokenFilePollInterval controls how often the token file is re-read.
	TokenFilePollInterval time.Duration `alloy:"token_file_poll_interval,attr,optional"`

	// ExpiryBuffer is subtracted from the token's expiry time to trigger proactive refresh.
	ExpiryBuffer time.Duration `alloy:"expiry_buffer,attr,optional"`

	// Header is the HTTP header name to set the token in.
	Header string `alloy:"header,attr,optional"`

	// Scheme is the authentication scheme prepended to the token value.
	Scheme string `alloy:"scheme,attr,optional"`

	// GCP contains settings for GCP workload identity federation.
	GCP *GCPArguments `alloy:"gcp,block,optional"`

	// Azure contains settings for Azure AD federated credentials.
	Azure *AzureArguments `alloy:"azure,block,optional"`

	// DebugMetrics configures component internal metrics.
	DebugMetrics otelcolCfg.DebugMetricsArguments `alloy:"debug_metrics,block,optional"`
}

// GCPArguments configures GCP workload identity federation via the STS endpoint,
// or GCP managed identity via the metadata server.
type GCPArguments struct {
	// ManagedIdentity enables GCP managed identity (metadata server) mode.
	// When true, tokens are fetched from the GCE/GKE metadata server instead of
	// performing STS token exchange. The token_file field is not required.
	ManagedIdentity bool `alloy:"managed_identity,attr,optional"`

	// Audience is the full resource name of the workload identity pool provider.
	// Required for workload identity (managed_identity = false).
	// Also required for managed identity when requesting ID tokens.
	Audience string `alloy:"audience,attr,optional"`

	// Scopes for the access token.
	Scopes []string `alloy:"scopes,attr,optional"`

	// SubjectTokenType is the type of the source token.
	SubjectTokenType string `alloy:"subject_token_type,attr,optional"`

	// RequestedTokenType is the type of token to request.
	// Use "urn:ietf:params:oauth:token-type:access_token" for opaque access tokens (default),
	// or "urn:ietf:params:oauth:token-type:id_token" for JWT ID tokens that can be validated
	// by downstream services (e.g., Istio RequestAuthentication).
	RequestedTokenType string `alloy:"requested_token_type,attr,optional"`

	// STSEndpoint overrides the GCP STS URL.
	STSEndpoint string `alloy:"sts_endpoint,attr,optional"`

	// IAMEndpoint overrides the GCP IAM Credentials API URL for service account impersonation.
	IAMEndpoint string `alloy:"iam_endpoint,attr,optional"`

	// MetadataEndpoint overrides the GCE metadata server base URL.
	// Only used when managed_identity is true.
	MetadataEndpoint string `alloy:"metadata_endpoint,attr,optional"`

	// MetadataServiceAccount is the service account to use when fetching tokens from
	// the metadata server. Only used when managed_identity is true.
	MetadataServiceAccount string `alloy:"metadata_service_account,attr,optional"`

	// ServiceAccountEmail enables IAM impersonation for the given service account.
	ServiceAccountEmail string `alloy:"service_account_email,attr,optional"`

	// ServiceAccountTokenLifetime is the duration for the impersonated token.
	ServiceAccountTokenLifetime time.Duration `alloy:"service_account_token_lifetime,attr,optional"`
}

// AzureArguments configures Azure AD federated credential token exchange,
// or Azure managed identity.
type AzureArguments struct {
	// ManagedIdentity enables Azure managed identity mode.
	// When true, tokens are fetched from the Azure Instance Metadata Service (IMDS)
	// instead of performing federated credential exchange.
	// The tenant_id field is not required in managed identity mode.
	ManagedIdentity bool `alloy:"managed_identity,attr,optional"`

	// TenantID is the Azure AD tenant ID.
	// Required for workload identity (managed_identity = false).
	TenantID string `alloy:"tenant_id,attr,optional"`

	// ClientID is the application (client) ID of the app registration.
	// For managed identity, set this to the client ID of a user-assigned managed identity.
	// Omit for system-assigned managed identity.
	ClientID string `alloy:"client_id,attr,optional"`

	// Cloud sets the Azure cloud environment.
	// Supported values: "public" (default), "government", "china".
	// Use authority_host for custom endpoints.
	Cloud string `alloy:"cloud,attr,optional"`

	// AuthorityHost overrides the Azure AD authority URL.
	// Use this for sovereign clouds or custom Azure AD endpoints.
	// Takes precedence over the cloud setting.
	AuthorityHost string `alloy:"authority_host,attr,optional"`

	// Scopes for the access token.
	Scopes []string `alloy:"scopes,attr"`
}

var _ auth.Arguments = Arguments{}

// SetToDefault implements syntax.Defaulter.
func (args *Arguments) SetToDefault() {
	args.TokenFilePollInterval = 5 * time.Minute
	args.ExpiryBuffer = time.Minute
	args.Header = "Authorization"
	args.Scheme = "Bearer"
	args.DebugMetrics.SetToDefault()
}

// isManagedIdentity returns true if the component is configured for managed identity mode.
func (args Arguments) isManagedIdentity() bool {
	if args.GCP != nil && args.GCP.ManagedIdentity {
		return true
	}
	if args.Azure != nil && args.Azure.ManagedIdentity {
		return true
	}
	return false
}

// Validate implements syntax.Validator.
func (args Arguments) Validate() error {
	if args.Provider == "" {
		return fmt.Errorf("provider must be set")
	}

	// token_file is required for workload identity, not for managed identity.
	if args.TokenFile == "" && !args.isManagedIdentity() {
		return fmt.Errorf("token_file must be set (unless managed_identity is enabled)")
	}

	switch args.Provider {
	case "gcp":
		if args.GCP == nil {
			return fmt.Errorf("gcp block is required when provider is \"gcp\"")
		}
		if args.Azure != nil {
			return fmt.Errorf("azure block must not be set when provider is \"gcp\"")
		}
		if !args.GCP.ManagedIdentity && args.GCP.Audience == "" {
			return fmt.Errorf("gcp.audience must be set for workload identity (or enable managed_identity)")
		}
		if args.GCP.ManagedIdentity && args.GCP.RequestedTokenType == "urn:ietf:params:oauth:token-type:id_token" && args.GCP.Audience == "" {
			return fmt.Errorf("gcp.audience must be set when requesting ID tokens with managed identity")
		}
	case "azure":
		if args.Azure == nil {
			return fmt.Errorf("azure block is required when provider is \"azure\"")
		}
		if args.GCP != nil {
			return fmt.Errorf("gcp block must not be set when provider is \"azure\"")
		}
		if !args.Azure.ManagedIdentity {
			if args.Azure.TenantID == "" {
				return fmt.Errorf("azure.tenant_id must be set for workload identity (or enable managed_identity)")
			}
			if args.Azure.ClientID == "" {
				return fmt.Errorf("azure.client_id must be set for workload identity (or enable managed_identity)")
			}
		}
		if len(args.Azure.Scopes) == 0 {
			return fmt.Errorf("azure.scopes must be set")
		}
	default:
		return fmt.Errorf("unsupported provider %q, must be \"gcp\" or \"azure\"", args.Provider)
	}

	return nil
}

func (args Arguments) buildConfig() *Config {
	cfg := &Config{
		Provider:              args.Provider,
		TokenFile:             args.TokenFile,
		TokenFilePollInterval: args.TokenFilePollInterval,
		ExpiryBuffer:          args.ExpiryBuffer,
		Header:                args.Header,
		Scheme:                args.Scheme,
		HTTPClient:            http.DefaultClient,
	}

	if args.GCP != nil {
		cfg.GCPManagedIdentity = args.GCP.ManagedIdentity
		cfg.GCPSTSEndpoint = args.GCP.STSEndpoint
		if cfg.GCPSTSEndpoint == "" {
			cfg.GCPSTSEndpoint = "https://sts.googleapis.com/v1/token"
		}
		cfg.GCPIAMEndpoint = args.GCP.IAMEndpoint
		if cfg.GCPIAMEndpoint == "" {
			cfg.GCPIAMEndpoint = "https://iamcredentials.googleapis.com"
		}
		cfg.GCPMetadataEndpoint = args.GCP.MetadataEndpoint
		if cfg.GCPMetadataEndpoint == "" {
			cfg.GCPMetadataEndpoint = "http://metadata.google.internal"
		}
		cfg.GCPMetadataServiceAccount = args.GCP.MetadataServiceAccount
		if cfg.GCPMetadataServiceAccount == "" {
			cfg.GCPMetadataServiceAccount = "default"
		}
		cfg.GCPAudience = args.GCP.Audience
		cfg.GCPScopes = args.GCP.Scopes
		if len(cfg.GCPScopes) == 0 {
			cfg.GCPScopes = []string{"https://www.googleapis.com/auth/cloud-platform"}
		}
		cfg.GCPSubjectTokenType = args.GCP.SubjectTokenType
		if cfg.GCPSubjectTokenType == "" {
			cfg.GCPSubjectTokenType = "urn:ietf:params:oauth:token-type:jwt"
		}
		cfg.GCPRequestedTokenType = args.GCP.RequestedTokenType
		if cfg.GCPRequestedTokenType == "" {
			cfg.GCPRequestedTokenType = "urn:ietf:params:oauth:token-type:access_token"
		}
		cfg.GCPServiceAccountEmail = args.GCP.ServiceAccountEmail
		cfg.GCPServiceAccountTokenLife = args.GCP.ServiceAccountTokenLifetime
		if cfg.GCPServiceAccountTokenLife == 0 {
			cfg.GCPServiceAccountTokenLife = time.Hour
		}
	}

	if args.Azure != nil {
		cfg.AzureManagedIdentity = args.Azure.ManagedIdentity
		cfg.AzureTenantID = args.Azure.TenantID
		cfg.AzureClientID = args.Azure.ClientID
		cfg.AzureScopes = args.Azure.Scopes
		cfg.AzureAuthorityHost = args.Azure.AuthorityHost
		if cfg.AzureAuthorityHost == "" {
			switch args.Azure.Cloud {
			case "government":
				cfg.AzureAuthorityHost = "https://login.microsoftonline.us"
			case "china":
				cfg.AzureAuthorityHost = "https://login.chinacloudapi.cn"
			default:
				// "public" or empty — use the default (Azure SDK default).
			}
		}
	}

	return cfg
}

// ConvertClient implements auth.Arguments.
func (args Arguments) ConvertClient() (otelcomponent.Config, error) {
	return &extensionConfig{}, nil
}

// ConvertServer implements auth.Arguments.
func (args Arguments) ConvertServer() (otelcomponent.Config, error) {
	return nil, nil
}

// Extensions implements auth.Arguments.
func (args Arguments) Extensions() map[otelcomponent.ID]otelcomponent.Component {
	return nil
}

// Exporters implements auth.Arguments.
func (args Arguments) Exporters() map[pipeline.Signal]map[otelcomponent.ID]otelcomponent.Component {
	return nil
}

// AuthFeatures implements auth.Arguments.
func (args Arguments) AuthFeatures() auth.AuthFeature {
	return auth.ClientAuthSupported
}

// DebugMetricsConfig implements auth.Arguments.
func (args Arguments) DebugMetricsConfig() otelcolCfg.DebugMetricsArguments {
	return args.DebugMetrics
}
