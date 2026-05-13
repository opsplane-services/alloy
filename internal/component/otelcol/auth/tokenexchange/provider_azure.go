package tokenexchange

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// azureProvider exchanges a Kubernetes service account token for an Azure AD access token
// using federated credential (client assertion with JWT bearer), or fetches a token
// using Azure managed identity (IMDS).
type azureProvider struct {
	scopes []string
	cred   azcore.TokenCredential
}

func newAzureProvider(cfg *Config) (*azureProvider, error) {
	p := &azureProvider{
		scopes: cfg.AzureScopes,
	}

	if cfg.AzureManagedIdentity {
		return p.initManagedIdentity(cfg)
	}
	return p.initWorkloadIdentity(cfg)
}

func azureClientOptions(authorityHost string) *azcore.ClientOptions {
	if authorityHost == "" {
		return nil
	}
	return &azcore.ClientOptions{
		Cloud: cloud.Configuration{
			ActiveDirectoryAuthorityHost: authorityHost,
		},
	}
}

func (p *azureProvider) initManagedIdentity(cfg *Config) (*azureProvider, error) {
	opts := &azidentity.ManagedIdentityCredentialOptions{}
	if cfg.AzureClientID != "" {
		opts.ID = azidentity.ClientID(cfg.AzureClientID)
	}
	if clientOpts := azureClientOptions(cfg.AzureAuthorityHost); clientOpts != nil {
		opts.ClientOptions = *clientOpts
	}

	cred, err := azidentity.NewManagedIdentityCredential(opts)
	if err != nil {
		return nil, fmt.Errorf("creating Azure managed identity credential: %w", err)
	}
	p.cred = cred
	return p, nil
}

func (p *azureProvider) initWorkloadIdentity(cfg *Config) (*azureProvider, error) {
	tokenFile := cfg.TokenFile

	var credOpts *azidentity.ClientAssertionCredentialOptions
	if clientOpts := azureClientOptions(cfg.AzureAuthorityHost); clientOpts != nil {
		credOpts = &azidentity.ClientAssertionCredentialOptions{
			ClientOptions: *clientOpts,
		}
	}

	// azidentity.NewClientAssertionCredential accepts a callback that returns the assertion token.
	// We read from the token file each time to pick up rotated tokens.
	cred, err := azidentity.NewClientAssertionCredential(
		cfg.AzureTenantID,
		cfg.AzureClientID,
		func(_ context.Context) (string, error) {
			data, err := os.ReadFile(tokenFile)
			if err != nil {
				return "", fmt.Errorf("reading token file %s: %w", tokenFile, err)
			}
			return string(data), nil
		},
		credOpts,
	)
	if err != nil {
		return nil, fmt.Errorf("creating Azure client assertion credential: %w", err)
	}
	p.cred = cred
	return p, nil
}

// Exchange uses the Azure credential to obtain an access token for the configured scopes.
func (p *azureProvider) Exchange(ctx context.Context, _ string) (string, time.Time, error) {
	token, err := p.cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: p.scopes,
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("Azure token exchange failed: %w", err)
	}

	return token.Token, token.ExpiresOn, nil
}
