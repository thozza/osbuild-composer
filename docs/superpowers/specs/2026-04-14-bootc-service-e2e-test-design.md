# Bootc Service End-to-End Functional Test Design

## Overview

A new end-to-end functional test for bootc composes that mimics the actual production service deployment. The test exercises the full pipeline: Cloud API with JWT auth, bootc compose with private registry, osbuild execution in an AWS EC2 executor, upload to target, and image verification.

## Motivation

Existing tests cover pieces of this flow but not the full production-like combination:
- `api.sh` tests traditional (non-bootc) composes via Cloud API with various upload targets, but does not use an executor and does not support bootc.
- `api-bootc.sh` tests bootc composes but focuses on the on-prem scenario (local upload target, no executor, no JWT).
- `worker-executor.sh` tests executor-based builds but uses the Weldr API (which does not support bootc).

No existing test covers: bootc compose + Cloud API + JWT + private registry + executor + cloud upload target + image verification.

## File Structure

### New files

- `test/cases/api-bootc-service.sh` -- main test driver
- `test/cases/api/bootc/guest.s3.sh` -- handler for `guest-image` + `aws.s3` (first image type)

### Modified files

- `Schutzfile` -- extended with bootc container ref mappings
- `.gitlab-ci.yml` -- new CI job
- `tools/provision.sh` -- already supports JWT mode (`provision.sh jwt`), no new file needed
- `test/cases/api/common/executor.sh` (new) -- extracted AWS EC2 executor setup helper

### Directory layout

```
test/cases/
  api.sh                        # traditional compose driver (unchanged)
  api-bootc.sh                  # on-prem bootc test (unchanged)
  api-bootc-service.sh          # bootc service compose driver (new)
  api/
    aws.sh, aws.s3.sh, ...      # handlers for api.sh (unchanged)
    common/                     # shared utilities
      common.sh                 # SSH, instance checks, customization verification
      aws.sh                    # AWS CLI install/setup
      s3.sh                     # S3 request creation, verifyDisk(), verifyEdgeCommit()
      executor.sh               # AWS EC2 executor setup (new)
    bootc/                      # handlers for api-bootc-service.sh (new)
      guest.s3.sh
```

## Schutzfile Bootc Container Ref Mapping

The Schutzfile is extended with a `bootc` key under each distro's `dependencies` section. The mapping is structured as: image-type -> arch -> ref-type -> full container reference (including registry host, image name, and tag).

```json
{
  "rhel-10.1": {
    "dependencies": {
      "osbuild": {
        "commit": "89601f77..."
      },
      "bootc": {
        "guest-image": {
          "x86_64": {
            "base": "quay.io/redhat-services-prod/insights-management-tenant/image-builder-bootc-foundry/rhel-10.1-qcow2:latest"
          },
          "aarch64": {
            "base": "quay.io/redhat-services-prod/insights-management-tenant/image-builder-bootc-foundry/rhel-10.1-qcow2:latest"
          }
        }
      }
    }
  }
}
```

- `base` is the primary container ref (the only one used initially).
- Future ref types (`build`, `installer`) can be added as siblings under the arch key without restructuring.
- The full container reference (registry + image + tag) is stored in the Schutzfile. This makes it easy to override with a test image from a different registry.

The test driver reads the ref directly:

```bash
BOOTC_CONTAINER_REF="${BOOTC_CONTAINER_REF_OVERRIDE:-$(jq -r \
  ".[\"${ID}-${VERSION_ID}\"].dependencies.bootc[\"${IMAGE_TYPE}\"][\"${ARCH}\"].base" \
  Schutzfile)}"
```

The optional `BOOTC_CONTAINER_REF_OVERRIDE` env var allows pointing the test at a different image/registry without modifying the Schutzfile.

The registry host for `podman login` is extracted from the ref. The registry credentials (`BOOTC_FOUNDRY_DERIVED_CONTAINERS_REGISTRY_USER`, `BOOTC_FOUNDRY_DERIVED_CONTAINERS_REGISTRY_PASS`) are provided as CI env vars.

## Test Driver Flow (`api-bootc-service.sh`)

The driver accepts one argument: `IMAGE_TYPE` (e.g., `guest-image`). It selects the matching handler script from `api/bootc/` using a `case` statement that maps each image type to its handler file and implicit upload target. For example, `guest-image` maps to `api/bootc/guest.s3.sh` (with `aws.s3` upload target). This is where the 1:1 image-type-to-target mapping is defined.

### Step 1: Setup and provisioning

- Source shared libs (`set-env-variables.sh`, `shared_lib.sh`)
- Provision with `provision.sh` (standard service scenario)
- Start PostgreSQL container for the DB queue, run tern migrations
- Source the handler script from `api/bootc/` based on `IMAGE_TYPE`
- Call `checkEnv()` to verify required env vars

### Step 2: JWT authentication and composer configuration

JWT setup is handled by calling `provision.sh jwt`, which already exists and provides:
- X.509 certificate generation
- JWT composer config template (`osbuild-composer-jwt.toml`) with `enable_jwt`, `jwt_keys_urls`, `jwt_tenant_provider_fields`
- JWT worker config template (`osbuild-worker-jwt.toml`) with `oauth_url`, `client_id`, `offline_token`
- Mock OpenID providers (`run-mock-auth-servers.sh`): HTTPS on port 8082 for composer, HTTP on port 8081 for worker
- Starting the correct systemd units: Cloud API socket + remote worker

After provisioning, the driver appends additional settings to `/etc/osbuild-composer/osbuild-composer.toml`:
- `[worker]` DB connection settings (PostgreSQL container from step 1)
- `[bootc] use_remote_container_source = true`

Restart `osbuild-composer`.

### Step 4: Worker configuration

- Create AWS EC2 keypair for executor
- Extract the registry host from `$BOOTC_CONTAINER_REF`
- Run `podman login --authfile /etc/osbuild-worker/containerauth.json` using:
  - `$BOOTC_FOUNDRY_DERIVED_CONTAINERS_REGISTRY_USER`
  - `$BOOTC_FOUNDRY_DERIVED_CONTAINERS_REGISTRY_PASS`
  - Registry host extracted from the container ref
- Create systemd drop-in for the worker service unit setting `Environment="REGISTRY_AUTH_FILE=/etc/osbuild-worker/containerauth.json"`
- Write `/etc/osbuild-worker/osbuild-worker.toml` with:
  - `[osbuild_executor]` type = `aws.ec2`, key_name = keypair from above
  - `[authentication]` oauth_url (HTTP mock provider), client_id, offline_token
- Restart worker service

No `[containers]` section in `osbuild-worker.toml`. The `REGISTRY_AUTH_FILE` env var from the systemd drop-in is sufficient for all container operations (BootcInfoResolveJob's podman calls, ContainerResolveJob, osbuild).

### Step 5: Executor provisioning

Extracted to a reusable helper `api/common/executor.sh` (following the established `worker-executor.sh` pattern) so both this test and `worker-executor.sh` can use it. The helper exposes `setupExecutor()` and `cleanupExecutor()` functions.

`setupExecutor()`:
- Wait for the executor EC2 instance to appear (tagged with parent instance ID)
- SSH into the executor
- Install osbuild and osbuild-composer packages from CI S3 repos (osbuild version pinned in Schutzfile, osbuild-composer from current CI commit)
- Open firewall port for worker-executor communication
- Start `osbuild-worker-executor`

`cleanupExecutor()`:
- Delete AWS keypair

### Step 6: Pre-compose verification

- Read the bootc container ref from Schutzfile (see Schutzfile section above)
- Verify the ref is NOT in local container storage: `podman image exists "$BOOTC_CONTAINER_REF"` (expect failure)

### Step 7: Compose execution

- `createReqFile` (from handler) -- writes the bootc compose request JSON
- `sendCompose` -- POST to Cloud API (`/api/image-builder-composer/v2/compose`) with JWT bearer token
- `waitForState` -- poll compose status until `success` or `failure`
- On failure: call `dump_db`, fetch worker journal logs, print diagnostics, exit 1

### Step 8: Post-compose verification

- Verify the bootc container ref IS now in local container storage: `podman image exists "$BOOTC_CONTAINER_REF"` (expect success, pulled by BootcInfoResolveJob)
- `checkUploadStatusOptions` (from handler) -- verify upload status fields
- `verify` (from handler) -- download and inspect the built image

### Step 9: Cleanup (trap EXIT)

- `cleanup` (from handler) -- remove cloud resources (S3 objects, etc.)
- `dump_db` -- save job results to artifacts
- Kill DB container
- Delete AWS keypair
- Kill background processes (mock OpenID provider, journal tail, etc.)

## Handler Script: `api/bootc/guest.s3.sh`

The handler sources `api/aws.s3.sh` to inherit most functions, then overrides only what differs for bootc.

### Inherited from `api/aws.s3.sh`

- `installClient()` -- AWS CLI setup
- `checkUploadStatusOptions()` -- verifies S3 URL contains expected bucket
- `verify()` -- downloads qcow2 from S3 presigned URL, calls `verifyDisk()` for offline inspection via `osbuild-image-info`
- `cleanup()` -- removes S3 objects

### Overridden

- `checkEnv()` -- verifies AWS credentials (`AWS_REGION`, `AWS_BUCKET`, `V2_AWS_ACCESS_KEY_ID`, `V2_AWS_SECRET_ACCESS_KEY`) plus bootc registry credentials (`BOOTC_FOUNDRY_DERIVED_CONTAINERS_REGISTRY_USER`, `BOOTC_FOUNDRY_DERIVED_CONTAINERS_REGISTRY_PASS`). Does not require `AWS_API_TEST_SHARE_ACCOUNT` (not needed for S3 target).
- `createReqFile()` -- writes the bootc-specific compose request:

```json
{
  "bootc": {
    "reference": "$BOOTC_CONTAINER_REF"
  },
  "image_request": {
    "architecture": "$ARCH",
    "image_type": "guest-image",
    "repositories": [],
    "upload_targets": [{
      "type": "aws.s3",
      "upload_options": {
        "region": "$AWS_REGION"
      }
    }]
  }
}
```

No customizations in the initial implementation. The request structure allows adding customizations later.

### Adding new image types

To add a new bootc image type (e.g., `ami` + `aws`):

1. Add the container ref mapping to Schutzfile under `bootc.<image-type>.base.<arch>`
2. Create a new handler `api/bootc/<image-type>.<target>.sh` (e.g., `api/bootc/ami.aws.sh`)
3. Source the appropriate non-bootc handler (e.g., `api/aws.sh`) and override `checkEnv()` and `createReqFile()`
4. Update the handler selection logic in `api-bootc-service.sh`
5. Optionally add a new CI job matrix entry

## CI Configuration

New job in `.gitlab-ci.yml`:

```yaml
API-bootc-service:
  stage: test
  extends: .terraform
  rules:
    - !reference [.upstream_and_ga_rules_all, rules]
  script:
    - schutzbot/deploy.sh
    - /usr/libexec/tests/osbuild-composer/api-bootc-service.sh guest-image
  variables:
    RUNNER: aws/rhel-10.1-ga-x86_64
    IAM_INSTANCE_PROFILE: worker-executor
```

- Runner: `aws/rhel-10.1-ga-x86_64`
- IAM profile: `worker-executor` (same as existing `WorkerExecutor` job, required for executor EC2 instance management)
- Registry credentials (`BOOTC_FOUNDRY_DERIVED_CONTAINERS_REGISTRY_USER`, `BOOTC_FOUNDRY_DERIVED_CONTAINERS_REGISTRY_PASS`) are defined as CI/CD variables in GitLab (masked/protected)
- Initially a single combination (guest-image); can be extended to a matrix later

## Key Design Decisions

1. **Standalone test file** (`api-bootc-service.sh`) rather than extending `api-bootc.sh` -- clean separation between on-prem and service test scenarios without risk of regressions.
2. **Handler pattern** matching `api.sh` -- pluggable image type verification via sourced scripts in `api/bootc/`.
3. **Handler reuse via sourcing** -- `api/bootc/guest.s3.sh` sources `api/aws.s3.sh` and overrides only `checkEnv()` and `createReqFile()`, minimizing duplication.
4. **`REGISTRY_AUTH_FILE` via systemd drop-in** instead of `[containers]` config in `osbuild-worker.toml` -- simpler, covers all container operations uniformly (BootcInfoResolveJob, ContainerResolveJob, osbuild).
5. **JWT from the start** -- the entire compose flow runs under JWT auth to test BootcPreManifest job in a multi-tenant context.
6. **Schutzfile for container ref pinning** -- consistent with existing dependency management; structured as image-type -> ref-type -> arch for future extensibility (`build`, `installer` ref types).
7. **Executor provisioned via SSH** (worker-executor.sh pattern) -- Packer-built AMIs are not available in PR pipelines (creation is skipped for PRs, no artifact passing mechanism exists).
8. **Offline image verification** (`osbuild-image-info`) for qcow2 initially -- consistent with existing `guest-image` + `aws.s3` behavior; pluggable verification allows boot-testing to be added later per image type.
9. **JWT setup via existing `provision.sh jwt`** -- no new helper needed; `provision.sh` already handles JWT certificates, config templates, mock OpenID providers, and systemd units. The test driver appends DB and bootc config after provisioning.
10. **Executor setup extracted to shared helper** -- avoids duplication between `api-bootc-service.sh` and `worker-executor.sh`; `worker-executor.sh` can adopt it later.
