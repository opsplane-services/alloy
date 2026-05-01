---
canonical: https://grafana.com/docs/alloy/latest/reference/components/otelcol/otelcol.auth.tokenexchange/
description: Learn about otelcol.auth.tokenexchange
labels:
  stage: public-preview
  products:
    - oss
title: otelcol.auth.tokenexchange
---

# `otelcol.auth.tokenexchange`

{{< docs/shared lookup="stability/public_preview.md" source="alloy" version="<ALLOY_VERSION>" >}}

`otelcol.auth.tokenexchange` exposes a `handler` that other `otelcol` components can use to authenticate requests using workload identity federation or managed identity.

This component supports two modes:

- **Workload identity**: Reads a source token from a file (for example, a Kubernetes projected service account token), exchanges it for a cloud provider access token, caches the result, and injects it into outgoing requests as a Bearer token.
- **Managed identity**: Fetches access tokens directly from the cloud provider's metadata service (GCE/GKE metadata server or Azure IMDS), suitable for VM-based deployments.

This component only supports client authentication.

Currently supported providers:
- **GCP**: Workload identity via [Security Token Service (STS)](https://cloud.google.com/iam/docs/reference/sts/rest/v1/TopLevel/token) with optional [service account impersonation](https://cloud.google.com/iam/docs/create-short-lived-credentials-direct), or managed identity via the [GCE metadata server](https://cloud.google.com/compute/docs/metadata/overview).
- **Azure**: Workload identity via [Azure AD federated credentials](https://learn.microsoft.com/en-us/entra/workload-id/workload-identity-federation) using client assertion with JWT bearer, or [managed identity](https://learn.microsoft.com/en-us/entra/identity/managed-identities-azure-resources/overview) via Azure IMDS.

You can specify multiple `otelcol.auth.tokenexchange` components by giving them different labels.

## Usage

### Workload identity (Kubernetes)

```alloy
otelcol.auth.tokenexchange "<LABEL>" {
  provider   = "<PROVIDER>"
  token_file = "<TOKEN_FILE_PATH>"

  gcp {
    audience = "<WORKLOAD_IDENTITY_POOL_PROVIDER>"
  }
}
```

### Managed identity (VM)

```alloy
otelcol.auth.tokenexchange "<LABEL>" {
  provider = "<PROVIDER>"

  azure {
    managed_identity = true
    scopes           = ["<SCOPE>"]
  }
}
```

## Arguments

You can use the following arguments with `otelcol.auth.tokenexchange`:

| Name                       | Type       | Description                                                                     | Default           | Required |
| -------------------------- | ---------- | ------------------------------------------------------------------------------- | ----------------- | -------- |
| `provider`                 | `string`   | The cloud provider to use. `"gcp"` or `"azure"`.                               |                   | yes      |
| `token_file`               | `string`   | Path to the source token file. Required for workload identity, not for managed identity. |            | no       |
| `header`                   | `string`   | HTTP header name to set the token in.                                           | `"Authorization"` | no       |
| `scheme`                   | `string`   | Authentication scheme prepended to the token value. Set to `""` to omit.        | `"Bearer"`        | no       |
| `expiry_buffer`            | `duration` | Duration before token expiry to trigger proactive refresh.                      | `"1m"`            | no       |
| `token_file_poll_interval` | `duration` | Interval for re-reading the source token file during refresh.                   | `"5m"`            | no       |

When sending the token, the value of `scheme` is prepended to the token value.
The resulting string is set as the value of the `header` HTTP header on every outgoing request.

Exactly one of the `gcp` or `azure` blocks must be configured, matching the `provider` value.

## Blocks

You can use the following blocks with `otelcol.auth.tokenexchange`:

| Block                            | Description                                                                | Required |
| -------------------------------- | -------------------------------------------------------------------------- | -------- |
| [`gcp`][gcp]                     | Configures GCP authentication (workload identity or managed identity).     | no       |
| [`azure`][azure]                 | Configures Azure authentication (workload identity or managed identity).   | no       |
| [`debug_metrics`][debug_metrics] | Configures the metrics that this component generates to monitor its state. | no       |

[gcp]: #gcp
[azure]: #azure
[debug_metrics]: #debug_metrics

### `gcp`

The `gcp` block configures GCP authentication. Set `managed_identity = true` to use the GCE/GKE metadata server, or configure `audience` for workload identity federation via the STS endpoint.

| Name                             | Type           | Description                                                                                 | Default                                                          | Required |
| -------------------------------- | -------------- | ------------------------------------------------------------------------------------------- | ---------------------------------------------------------------- | -------- |
| `managed_identity`               | `bool`         | Use GCE/GKE metadata server instead of STS token exchange.                                  | `false`                                                          | no       |
| `audience`                       | `string`       | Full resource name of the workload identity pool provider. Required for workload identity. Also required for managed identity when requesting ID tokens. |         | no       |
| `scopes`                         | `list(string)` | OAuth scopes for the access token.                                                          | `["https://www.googleapis.com/auth/cloud-platform"]`            | no       |
| `subject_token_type`             | `string`       | The type of the source token. Only used for workload identity.                              | `"urn:ietf:params:oauth:token-type:jwt"`                        | no       |
| `requested_token_type`           | `string`       | The type of token to request. Use `id_token` type to get a JWT for service-to-service auth. Works with both workload and managed identity. | `"urn:ietf:params:oauth:token-type:access_token"` | no |
| `sts_endpoint`                   | `string`       | Override the GCP STS endpoint URL. Only used for workload identity.                         | `"https://sts.googleapis.com/v1/token"`                         | no       |
| `iam_endpoint`                   | `string`       | Override the GCP IAM Credentials API base URL for service account impersonation.            | `"https://iamcredentials.googleapis.com"`                       | no       |
| `metadata_endpoint`              | `string`       | Override the GCE metadata server base URL. Only used for managed identity.                  | `"http://metadata.google.internal"`                             | no       |
| `metadata_service_account`       | `string`       | The service account to use with the metadata server. Only used for managed identity.        | `"default"`                                                     | no       |
| `service_account_email`          | `string`       | If set, enables IAM impersonation for this service account after STS exchange. Only used for workload identity. |                                                    | no       |
| `service_account_token_lifetime` | `duration`     | Lifetime of the impersonated service account token.                                         | `"1h"`                                                          | no       |

**Workload identity mode** (`managed_identity = false`, default):
The `audience` field is required. Set `requested_token_type` to `"urn:ietf:params:oauth:token-type:id_token"` to receive a JWT ID token instead of an opaque access token. ID tokens can be validated by downstream services using Istio RequestAuthentication or any OIDC-compatible server-side auth.

The `audience` must be the full resource name of a workload identity pool provider, for example:
```
//iam.googleapis.com/projects/PROJECT_NUMBER/locations/global/workloadIdentityPools/POOL_ID/providers/PROVIDER_ID
```

**Managed identity mode** (`managed_identity = true`):
The component fetches tokens from the metadata server. By default it uses the `default` service account. Set `metadata_service_account` to use a different service account on VMs with multiple service accounts. Set `requested_token_type` to `"urn:ietf:params:oauth:token-type:id_token"` and provide `audience` to get a JWT ID token from the `/identity` endpoint instead of an opaque access token from `/token`.

### `azure`

The `azure` block configures Azure AD authentication. Set `managed_identity = true` to use Azure managed identity (IMDS), or configure `tenant_id` and `client_id` for workload identity federation.

| Name               | Type           | Description                                                                            | Default    | Required |
| ------------------ | -------------- | -------------------------------------------------------------------------------------- | ---------- | -------- |
| `managed_identity` | `bool`         | Use Azure managed identity (IMDS) instead of federated credential exchange.            | `false`    | no       |
| `tenant_id`        | `string`       | The Azure AD tenant ID. Required when `managed_identity` is `false`.                   |            | no       |
| `client_id`        | `string`       | The application (client) ID. Required for workload identity. For managed identity, set this for user-assigned identity; omit for system-assigned. |  | no       |
| `cloud`            | `string`       | Azure cloud environment: `"public"` (default), `"government"`, or `"china"`.           | `"public"` | no       |
| `authority_host`   | `string`       | Override the Azure AD authority URL. Takes precedence over `cloud`.                    |            | no       |
| `scopes`           | `list(string)` | OAuth scopes for the access token.                                                     |            | yes      |

The `scopes` typically use the `/.default` suffix, for example `["https://monitor.azure.com/.default"]`.

When `managed_identity` is `true`, the component fetches tokens from the [Azure Instance Metadata Service (IMDS)](https://learn.microsoft.com/en-us/entra/identity/managed-identities-azure-resources/how-to-use-vm-token). If `client_id` is set, it uses a user-assigned managed identity; otherwise it uses the system-assigned managed identity.

For sovereign clouds, set `cloud` to the appropriate value or use `authority_host` for a custom endpoint:
- **Azure Government**: `cloud = "government"` (uses `https://login.microsoftonline.us`)
- **Azure China**: `cloud = "china"` (uses `https://login.chinacloudapi.cn`)

### `debug_metrics`

{{< docs/shared lookup="reference/components/otelcol-debug-metrics-block.md" source="alloy" version="<ALLOY_VERSION>" >}}

## Exported fields

The following fields are exported and can be referenced by other components:

| Name      | Type                       | Description                                                     |
| --------- | -------------------------- | --------------------------------------------------------------- |
| `handler` | `capsule(otelcol.Handler)` | A value that other components can use to authenticate requests. |

## Component health

`otelcol.auth.tokenexchange` is reported as unhealthy if the initial token exchange fails or if given an invalid configuration.

## Debug information

`otelcol.auth.tokenexchange` doesn't expose any component-specific debug information.

## Examples

### GCP workload identity federation (Kubernetes)

Exchange a Kubernetes projected service account token for a GCP access token using workload identity federation with service account impersonation.

This approach is used for cross-cloud federation (e.g., non-GKE clusters federating with GCP) where you need an explicit projected service account token with a specific audience. The projected volume must be configured in the Deployment:

```alloy
otelcol.auth.tokenexchange "gcp_wif" {
  provider   = "gcp"
  token_file = "/var/run/secrets/tokens/gcp-token"

  gcp {
    audience              = "//iam.googleapis.com/projects/123456789/locations/global/workloadIdentityPools/my-pool/providers/my-provider"
    service_account_email = "my-sa@my-project.iam.gserviceaccount.com"
  }
}

otelcol.exporter.otlp "default" {
  client {
    endpoint = env("OTLP_ENDPOINT")
    auth     = otelcol.auth.tokenexchange.gcp_wif.handler
  }
}
```

The Deployment needs a projected service account token volume with the matching audience:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: alloy
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: alloy
spec:
  template:
    metadata:
      labels:
        app: alloy
    spec:
      serviceAccountName: alloy
      containers:
      - name: alloy
        image: grafana/alloy:latest
        volumeMounts:
        - name: gcp-token
          mountPath: /var/run/secrets/tokens
          readOnly: true
      volumes:
      - name: gcp-token
        projected:
          sources:
          - serviceAccountToken:
              audience: "//iam.googleapis.com/projects/123456789/locations/global/workloadIdentityPools/my-pool/providers/my-provider"
              expirationSeconds: 3600
              path: gcp-token
```

{{< admonition type="note" >}}
On GKE with native workload identity, use `managed_identity = true` instead. GKE handles token exchange transparently via the metadata server, so no projected volume is needed.
{{< /admonition >}}

### GCP workload identity federation without impersonation

If your workload identity pool grants direct access without service account impersonation, omit the `service_account_email` field:

```alloy
otelcol.auth.tokenexchange "gcp_direct" {
  provider   = "gcp"
  token_file = "/var/run/secrets/tokens/gcp-token"

  gcp {
    audience = "//iam.googleapis.com/projects/123456789/locations/global/workloadIdentityPools/my-pool/providers/my-provider"
  }
}

otelcol.exporter.otlp "default" {
  client {
    endpoint = env("OTLP_ENDPOINT")
    auth     = otelcol.auth.tokenexchange.gcp_direct.handler
  }
}
```

### GCP managed identity - access token (VM)

Use the GCE metadata server on a Compute Engine VM or GKE node to fetch access tokens using the VM's service account:

```alloy
otelcol.auth.tokenexchange "gcp_vm" {
  provider = "gcp"

  gcp {
    managed_identity = true
  }
}

otelcol.exporter.otlp "default" {
  client {
    endpoint = env("OTLP_ENDPOINT")
    auth     = otelcol.auth.tokenexchange.gcp_vm.handler
  }
}
```

### GCP managed identity - ID token for federated auth (VM)

Fetch a JWT ID token from the GCE metadata server that can be validated by a receiving service using Istio RequestAuthentication. Requires `audience` to be set to the target service identifier:

```alloy
otelcol.auth.tokenexchange "gcp_vm_jwt" {
  provider = "gcp"

  gcp {
    managed_identity     = true
    requested_token_type = "urn:ietf:params:oauth:token-type:id_token"
    audience             = "https://my-target-service.example.com"
  }
}

otelcol.exporter.otlphttp "default" {
  client {
    endpoint = env("OTLP_ENDPOINT")
    auth     = otelcol.auth.tokenexchange.gcp_vm_jwt.handler
  }
}
```

On the receiving side, validate the JWT with Istio:

```yaml
apiVersion: security.istio.io/v1
kind: RequestAuthentication
metadata:
  name: gcp-vm-jwt
spec:
  jwtRules:
  - issuer: "https://accounts.google.com"
    jwksUri: "https://www.googleapis.com/oauth2/v3/certs"
    audiences:
    - "https://my-target-service.example.com"
```

### GCP managed identity with specific service account

On VMs with multiple service accounts, specify which one to use:

```alloy
otelcol.auth.tokenexchange "gcp_vm_sa" {
  provider = "gcp"

  gcp {
    managed_identity         = true
    metadata_service_account = "my-sa@my-project.iam.gserviceaccount.com"
  }
}

otelcol.exporter.otlp "default" {
  client {
    endpoint = env("OTLP_ENDPOINT")
    auth     = otelcol.auth.tokenexchange.gcp_vm_sa.handler
  }
}
```

### Azure workload identity federation using environment variables (Kubernetes)

On AKS with workload identity enabled, the Azure workload identity webhook automatically injects the following environment variables into pods:
- `AZURE_TENANT_ID`
- `AZURE_CLIENT_ID`
- `AZURE_FEDERATED_TOKEN_FILE`
- `AZURE_AUTHORITY_HOST`

You can use `sys.env()` to reference these directly in the Alloy configuration:

```alloy
otelcol.auth.tokenexchange "azure_wif" {
  provider   = "azure"
  token_file = sys.env("AZURE_FEDERATED_TOKEN_FILE")

  azure {
    tenant_id      = sys.env("AZURE_TENANT_ID")
    client_id      = sys.env("AZURE_CLIENT_ID")
    authority_host = sys.env("AZURE_AUTHORITY_HOST")
    scopes         = ["https://monitor.azure.com/.default"]
  }
}

otelcol.exporter.otlphttp "default" {
  client {
    endpoint = env("OTLP_ENDPOINT")
    auth     = otelcol.auth.tokenexchange.azure_wif.handler
  }
}
```

The Deployment only needs the `azure.workload.identity/use: "true"` label. The AKS workload identity webhook automatically injects the environment variables, the projected token volume, and the volume mount into the pod. No manual volume configuration is required:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: alloy
  annotations:
    azure.workload.identity/client-id: "yyyyyyyy-yyyy-yyyy-yyyy-yyyyyyyyyyyy"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: alloy
spec:
  template:
    metadata:
      labels:
        app: alloy
        azure.workload.identity/use: "true"
    spec:
      serviceAccountName: alloy
      containers:
      - name: alloy
        image: grafana/alloy:latest
        # The webhook automatically injects:
        # - AZURE_TENANT_ID, AZURE_CLIENT_ID, AZURE_FEDERATED_TOKEN_FILE,
        #   AZURE_AUTHORITY_HOST environment variables
        # - A projected service account token volume at the
        #   AZURE_FEDERATED_TOKEN_FILE path
```

### Azure workload identity with explicit values (Kubernetes)

If you prefer to set the values explicitly instead of using environment variables:

```alloy
otelcol.auth.tokenexchange "azure_wif" {
  provider   = "azure"
  token_file = "/var/run/secrets/azure/tokens/azure-identity-token"

  azure {
    tenant_id = "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
    client_id = "yyyyyyyy-yyyy-yyyy-yyyy-yyyyyyyyyyyy"
    scopes    = ["https://monitor.azure.com/.default"]
  }
}

otelcol.exporter.otlphttp "default" {
  client {
    endpoint = env("OTLP_ENDPOINT")
    auth     = otelcol.auth.tokenexchange.azure_wif.handler
  }
}
```

### Azure managed identity - system-assigned (VM)

Use the system-assigned managed identity on an Azure VM or VMSS. No `tenant_id` or `client_id` is needed:

```alloy
otelcol.auth.tokenexchange "azure_vm" {
  provider = "azure"

  azure {
    managed_identity = true
    scopes           = ["https://monitor.azure.com/.default"]
  }
}

otelcol.exporter.otlphttp "default" {
  client {
    endpoint = env("OTLP_ENDPOINT")
    auth     = otelcol.auth.tokenexchange.azure_vm.handler
  }
}
```

### Azure managed identity - user-assigned (VM)

Use a user-assigned managed identity by specifying the `client_id`:

```alloy
otelcol.auth.tokenexchange "azure_vm_ua" {
  provider = "azure"

  azure {
    managed_identity = true
    client_id        = "yyyyyyyy-yyyy-yyyy-yyyy-yyyyyyyyyyyy"
    scopes           = ["https://monitor.azure.com/.default"]
  }
}

otelcol.exporter.otlphttp "default" {
  client {
    endpoint = env("OTLP_ENDPOINT")
    auth     = otelcol.auth.tokenexchange.azure_vm_ua.handler
  }
}
```

### GCP ID token for service-to-service auth (Istio)

Request a JWT ID token instead of an opaque access token for server-side validation with workload identity:

```alloy
otelcol.auth.tokenexchange "gcp_jwt" {
  provider   = "gcp"
  token_file = "/var/run/secrets/tokens/gcp-token"

  gcp {
    audience             = "//iam.googleapis.com/projects/123456789/locations/global/workloadIdentityPools/my-pool/providers/my-provider"
    requested_token_type = "urn:ietf:params:oauth:token-type:id_token"
  }
}

otelcol.exporter.otlphttp "default" {
  client {
    endpoint = env("OTLP_ENDPOINT")
    auth     = otelcol.auth.tokenexchange.gcp_jwt.handler
  }
}
```

On the receiving cluster, Istio RequestAuthentication can validate the JWT:

```yaml
apiVersion: security.istio.io/v1
kind: RequestAuthentication
metadata:
  name: gcp-wif-auth
spec:
  jwtRules:
  - issuer: "https://sts.googleapis.com"
    jwksUri: "https://www.googleapis.com/oauth2/v3/certs"
```

### Azure federated auth with Istio server-side validation

Azure AD access tokens are JWTs and can be validated by Istio on the receiving side. This works for both workload identity and managed identity:

```alloy
otelcol.auth.tokenexchange "azure_fed" {
  provider   = "azure"
  token_file = sys.env("AZURE_FEDERATED_TOKEN_FILE")

  azure {
    tenant_id = sys.env("AZURE_TENANT_ID")
    client_id = sys.env("AZURE_CLIENT_ID")
    scopes    = ["api://target-app-id/.default"]
  }
}

otelcol.exporter.otlphttp "default" {
  client {
    endpoint = env("OTLP_ENDPOINT")
    auth     = otelcol.auth.tokenexchange.azure_fed.handler
  }
}
```

On the receiving cluster, validate the Azure AD JWT with Istio:

```yaml
apiVersion: security.istio.io/v1
kind: RequestAuthentication
metadata:
  name: azure-ad-jwt
spec:
  jwtRules:
  - issuer: "https://sts.windows.net/TENANT_ID/"
    jwksUri: "https://login.microsoftonline.com/TENANT_ID/discovery/v2.0/keys"
    audiences:
    - "api://target-app-id"
```

### Azure managed identity with Istio server-side validation

Azure managed identity tokens are also JWTs and can be validated by Istio:

```alloy
otelcol.auth.tokenexchange "azure_vm_fed" {
  provider = "azure"

  azure {
    managed_identity = true
    scopes           = ["api://target-app-id/.default"]
  }
}

otelcol.exporter.otlphttp "default" {
  client {
    endpoint = env("OTLP_ENDPOINT")
    auth     = otelcol.auth.tokenexchange.azure_vm_fed.handler
  }
}
```

On the receiving cluster:

```yaml
apiVersion: security.istio.io/v1
kind: RequestAuthentication
metadata:
  name: azure-mi-jwt
spec:
  jwtRules:
  - issuer: "https://sts.windows.net/TENANT_ID/"
    jwksUri: "https://login.microsoftonline.com/TENANT_ID/discovery/v2.0/keys"
    audiences:
    - "api://target-app-id"
```

### Azure Government cloud

Use Azure Government sovereign cloud endpoints:

```alloy
otelcol.auth.tokenexchange "azure_gov" {
  provider   = "azure"
  token_file = sys.env("AZURE_FEDERATED_TOKEN_FILE")

  azure {
    tenant_id = sys.env("AZURE_TENANT_ID")
    client_id = sys.env("AZURE_CLIENT_ID")
    cloud     = "government"
    scopes    = ["https://monitor.azure.us/.default"]
  }
}

otelcol.exporter.otlphttp "default" {
  client {
    endpoint = env("OTLP_ENDPOINT")
    auth     = otelcol.auth.tokenexchange.azure_gov.handler
  }
}
```

### Custom header and scheme

Use a custom HTTP header or scheme for the token injection:

```alloy
otelcol.auth.tokenexchange "custom_header" {
  provider   = "azure"
  token_file = sys.env("AZURE_FEDERATED_TOKEN_FILE")
  header     = "X-Custom-Auth"
  scheme     = ""

  azure {
    tenant_id = sys.env("AZURE_TENANT_ID")
    client_id = sys.env("AZURE_CLIENT_ID")
    scopes    = ["https://monitor.azure.com/.default"]
  }
}
```

This sets `X-Custom-Auth: <raw-token>` on outgoing requests instead of the default `Authorization: Bearer <token>`.

### Custom refresh tuning

Adjust the token refresh behavior:

```alloy
otelcol.auth.tokenexchange "gcp_custom" {
  provider                 = "gcp"
  token_file               = "/var/run/secrets/tokens/gcp-token"
  expiry_buffer            = "2m"
  token_file_poll_interval = "10m"

  gcp {
    audience                       = "//iam.googleapis.com/projects/123456789/locations/global/workloadIdentityPools/my-pool/providers/my-provider"
    service_account_email          = "my-sa@my-project.iam.gserviceaccount.com"
    service_account_token_lifetime = "2h"
  }
}
```

[otelcol.exporter.otlp]: ../otelcol.exporter.otlp/
[otelcol.exporter.otlphttp]: ../otelcol.exporter.otlphttp/
