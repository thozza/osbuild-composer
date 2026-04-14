# Bootc Service End-to-End Functional Test Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create a new end-to-end functional test (`api-bootc-service.sh`) for bootc composes that exercises the full production-like pipeline: Cloud API + JWT + private registry + AWS EC2 executor + S3 upload + image verification.

**Architecture:** A standalone test driver (`test/cases/api-bootc-service.sh`) provisions composer with JWT auth and bootc remote container sources, configures the worker with executor and registry auth, then triggers a bootc compose via Cloud API and verifies the result. Image-type-specific logic lives in handler scripts under `test/cases/api/bootc/` that source and override existing non-bootc handlers. Shared helpers for executor setup are extracted into `test/cases/api/common/executor.sh`.

**Tech Stack:** Bash (test scripts), PostgreSQL (job queue), AWS EC2 (executor), AWS S3 (upload target), JWT (authentication), podman (container registry auth)

**Spec:** `docs/superpowers/specs/2026-04-14-bootc-service-e2e-test-design.md`

---

## File Map

| File | Action | Purpose |
|------|--------|---------|
| `test/cases/api/common/executor.sh` | Create | Shared executor setup/cleanup functions |
| `test/cases/api/bootc/guest.s3.sh` | Create | Handler for guest-image + aws.s3 bootc compose |
| `test/cases/api-bootc-service.sh` | Create | Main test driver |
| `Schutzfile` | Modify | Add bootc container ref mappings |
| `.gitlab-ci.yml` | Modify | Add CI job |

---

### Task 1: Extract Executor Setup Helper

Extract the executor provisioning logic from `test/cases/worker-executor.sh` into a reusable helper. This is foundational — the main test driver will call these functions.

**Files:**
- Create: `test/cases/api/common/executor.sh`
- Reference: `test/cases/worker-executor.sh` (lines 65-179 for the pattern)

- [ ] **Step 1: Create `test/cases/api/common/executor.sh`**

This helper expects the following variables to be set by the caller:
- `INSTANCE_ID` — parent EC2 instance ID (for tagging)
- `AWS_CMD` — AWS CLI command (may be containerized)
- `CONTAINER_RUNTIME` — podman or docker
- `CONTAINER_IMAGE_CLOUD_TOOLS` — cloud tools container image
- `KILL_PIDS` — array for background process PIDs
- `TEMPDIR` or `WORKDIR` — temp directory for keypair storage

It also uses `GIT_COMMIT`, `CI_COMMIT_SHA`, `ID`, `VERSION_ID`, `ARCH` from the environment (set by `set-env-variables.sh`).

```bash
#!/usr/bin/bash

source /usr/libexec/tests/osbuild-composer/shared_lib.sh

EXECUTOR_KEYPAIR=""
EXECUTOR_IP=""

function setupExecutorAWSClient() {
    if ! hash aws; then
        echo "Using 'awscli' from a container"
        sudo "${CONTAINER_RUNTIME}" pull "${CONTAINER_IMAGE_CLOUD_TOOLS}"

        AWS_CMD="sudo ${CONTAINER_RUNTIME} run --rm \
            -v ${WORKDIR}:${WORKDIR}:Z \
            ${CONTAINER_IMAGE_CLOUD_TOOLS} aws --region $AWS_REGION --output json --color on"
    else
        echo "Using pre-installed 'aws' from the system"
        AWS_CMD="aws --region $AWS_REGION --output json --color on"
    fi
    $AWS_CMD --version
}

function setupExecutorKeypair() {
    EXECUTOR_KEYPAIR="${WORKDIR}/executor-keypair.pem"
    INSTANCE_ID=$(curl -Ls http://169.254.169.254/latest/meta-data/instance-id)

    $AWS_CMD ec2 create-key-pair \
        --key-name "key-for-${INSTANCE_ID}-executor" \
        --query 'KeyMaterial' \
        --output text > "$EXECUTOR_KEYPAIR"
    chmod 400 "$EXECUTOR_KEYPAIR"
    $AWS_CMD ec2 describe-key-pairs --key-names "key-for-${INSTANCE_ID}-executor"
}

function cleanupExecutorKeypair() {
    AWS_CMD="${AWS_CMD:-}"
    if [ -n "$AWS_CMD" ] && [ -f "${EXECUTOR_KEYPAIR}" ]; then
        $AWS_CMD ec2 delete-key-pair --key-name "key-for-${INSTANCE_ID}-executor"
    fi
}

function waitForExecutorInstance() {
    local DESCR_INST="${WORKDIR}/descr-inst.json"

    EXECUTOR_IP=0
    for _ in {1..60}; do
        $AWS_CMD ec2 describe-instances \
            --filter "Name=tag:parent,Values=$INSTANCE_ID" > "$DESCR_INST"
        RESERVATIONS=$(jq -r '.Reservations | length' "$DESCR_INST")
        if [ "$RESERVATIONS" -gt 0 ]; then
            EXECUTOR_IP=$(jq -r '.Reservations[0].Instances[0].PrivateIpAddress' "$DESCR_INST")
            break
        fi

        echo "Reservation not ready yet, waiting..."
        sleep 60
    done

    if [ "$EXECUTOR_IP" = 0 ]; then
        redprint "Unable to find executor host"
        exit 1
    fi

    local RDY=0
    for _ in {0..60}; do
        if ssh-keyscan "$EXECUTOR_IP" > /dev/null 2>&1; then
            RDY=1
            break
        fi
        sleep 10
    done

    if [ "$RDY" = 0 ]; then
        redprint "Unable to reach executor host $EXECUTOR_IP"
        exit 1
    fi

    greenprint "Executor instance is ready at $EXECUTOR_IP"
}

function provisionExecutor() {
    local GIT_COMMIT="${GIT_COMMIT:-${CI_COMMIT_SHA}}"
    local OSBUILD_GIT_COMMIT
    OSBUILD_GIT_COMMIT=$(cat Schutzfile | jq -r '.["'"${ID}-${VERSION_ID}"'"].dependencies.osbuild.commit')

    # Install osbuild and osbuild-composer packages from CI S3 repos
    # shellcheck disable=SC2087
    ssh -oStrictHostKeyChecking=no -i "$EXECUTOR_KEYPAIR" "fedora@$EXECUTOR_IP" sudo tee "/etc/yum.repos.d/osbuild.repo" <<EOF
[osbuild-composer]
name=osbuild-composer
baseurl=http://osbuild-composer-repos.s3-website.us-east-2.amazonaws.com/osbuild-composer/${ID}-${VERSION_ID}/${ARCH}/${GIT_COMMIT}
enabled=1
gpgcheck=0
priority=10
[osbuild]
name=osbuild
baseurl=http://osbuild-composer-repos.s3-website.us-east-2.amazonaws.com/osbuild/${ID}-${VERSION_ID}/${ARCH}/${OSBUILD_GIT_COMMIT}
enabled=1
gpgcheck=0
priority=10
EOF

    ssh -oStrictHostKeyChecking=no -i "$EXECUTOR_KEYPAIR" "fedora@$EXECUTOR_IP" \
        sudo dnf install -y osbuild-composer osbuild

    greenprint "Opening worker-executor port on firewall"
    ssh -oStrictHostKeyChecking=no -i "$EXECUTOR_KEYPAIR" "fedora@$EXECUTOR_IP" \
        sudo firewall-cmd --zone=public --add-port=8001/tcp --permanent || true
    ssh -oStrictHostKeyChecking=no -i "$EXECUTOR_KEYPAIR" "fedora@$EXECUTOR_IP" \
        sudo firewall-cmd --reload || true
}

function startExecutor() {
    greenprint "Starting worker executor"
    ssh -oStrictHostKeyChecking=no -i "$EXECUTOR_KEYPAIR" "fedora@$EXECUTOR_IP" \
        sudo /usr/libexec/osbuild-composer/osbuild-worker-executor -host 0.0.0.0 &
    KILL_PIDS+=("$!")
}

function setupExecutor() {
    setupExecutorKeypair
    waitForExecutorInstance
    provisionExecutor
    startExecutor
}
```

- [ ] **Step 2: Verify syntax and lint**

Run:
```bash
bash -n test/cases/api/common/executor.sh
shellcheck test/cases/api/common/executor.sh
```
Expected: No errors. Fix any shellcheck warnings before proceeding.

- [ ] **Step 3: Commit**

```bash
git add test/cases/api/common/executor.sh
git commit -m "test: extract executor setup into shared helper

Allow the new bootc service test and worker-executor.sh
to share executor provisioning logic instead of
duplicating the keypair, instance wait, and SSH setup."
```

---

### Task 2: Create Bootc Handler for guest-image + aws.s3

Create the handler script that sources the existing `api/aws.s3.sh` and overrides only what differs for bootc composes.

**Files:**
- Create: `test/cases/api/bootc/guest.s3.sh`
- Reference: `test/cases/api/aws.s3.sh` (sourced for inherited functions)

- [ ] **Step 1: Create directory and handler file**

```bash
mkdir -p test/cases/api/bootc
```

Write `test/cases/api/bootc/guest.s3.sh`:

```bash
#!/usr/bin/bash

# Handler for bootc guest-image compose with aws.s3 upload target.
# Sources the non-bootc aws.s3 handler to inherit installClient(),
# checkUploadStatusOptions(), verify(), and cleanup(). Overrides
# checkEnv() and createReqFile() for bootc-specific behavior.

source /usr/libexec/tests/osbuild-composer/api/aws.s3.sh

# Override: check env vars needed for bootc S3 compose
function checkEnv() {
    printenv AWS_REGION AWS_BUCKET V2_AWS_ACCESS_KEY_ID V2_AWS_SECRET_ACCESS_KEY > /dev/null
    printenv BOOTC_FOUNDRY_DERIVED_CONTAINERS_REGISTRY_USER BOOTC_FOUNDRY_DERIVED_CONTAINERS_REGISTRY_PASS > /dev/null
}

# Override: create bootc-specific compose request
function createReqFile() {
    cat > "$REQUEST_FILE" << EOF
{
  "bootc": {
    "reference": "$BOOTC_CONTAINER_REF"
  },
  "image_request": {
    "architecture": "$ARCH",
    "image_type": "${IMAGE_TYPE}",
    "repositories": [],
    "upload_targets": [
      {
        "type": "aws.s3",
        "upload_options": {
          "region": "${AWS_REGION}"
        }
      }
    ]
  }
}
EOF
}
```

- [ ] **Step 2: Verify syntax and lint**

Run:
```bash
bash -n test/cases/api/bootc/guest.s3.sh
shellcheck test/cases/api/bootc/guest.s3.sh
```
Expected: No errors. Fix any shellcheck warnings before proceeding.

- [ ] **Step 3: Commit**

```bash
git add test/cases/api/bootc/guest.s3.sh
git commit -m "test: add bootc handler for guest-image + aws.s3

Reuse the existing S3 verification and cleanup from
aws.s3.sh, only overriding the compose request to use
the bootc API shape instead of the traditional one."
```

---

### Task 3: Add Bootc Container Ref Mapping to Schutzfile

Add the `bootc` dependency mapping under `rhel-10.1` in the Schutzfile.

**Files:**
- Modify: `Schutzfile` (lines 175-181, the `rhel-10.1` entry)

- [ ] **Step 1: Add bootc mapping to rhel-10.1**

In `Schutzfile`, change the `rhel-10.1` entry from:

```json
  "rhel-10.1": {
    "dependencies": {
      "osbuild": {
        "commit": "89601f77042d28dcd7a4b192c901d94ffea8d251"
      }
    }
  },
```

To:

```json
  "rhel-10.1": {
    "dependencies": {
      "osbuild": {
        "commit": "89601f77042d28dcd7a4b192c901d94ffea8d251"
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
  },
```

- [ ] **Step 2: Verify JSON validity**

Run: `python3 -m json.tool Schutzfile > /dev/null`
Expected: No output (valid JSON)

- [ ] **Step 3: Commit**

```bash
git add Schutzfile
git commit -m "test: pin bootc container refs in Schutzfile

Keep bootc image refs in one place so the e2e test
can look them up by distro/image-type/arch, and so
we can update them independently of the test code."
```

---

### Task 4: Create the Main Test Driver

This is the core of the test. The driver orchestrates the full flow: provisioning, JWT auth, composer/worker config, executor setup, compose execution, and verification.

**Files:**
- Create: `test/cases/api-bootc-service.sh`
- Reference: `test/cases/api-bootc.sh` (bootc compose request/wait pattern)
- Reference: `test/cases/api.sh` (handler sourcing pattern, JWT setup pattern)
- Reference: `test/cases/worker-executor.sh` (executor provisioning pattern)
- Reference: `tools/provision.sh` (JWT provisioning)

- [ ] **Step 1: Create `test/cases/api-bootc-service.sh`**

The test driver uses `provision.sh jwt` which handles:
- Certificate generation
- Copying JWT composer/worker TOML configs
- Starting mock auth servers (HTTPS on 8082, HTTP on 8081)
- Starting the right systemd units (remote worker, Cloud API socket)

After provisioning, the driver appends DB, bootc, and executor config.

```bash
#!/usr/bin/bash

#
# End-to-end functional test for bootc composes in a service deployment.
# Tests the full pipeline: Cloud API + JWT + private registry + AWS EC2
# executor + cloud upload target + image verification.
#
# Usage: api-bootc-service.sh <image-type>
#   e.g.: api-bootc-service.sh guest-image
#

set -euo pipefail

source /usr/libexec/osbuild-composer-test/set-env-variables.sh
source /usr/libexec/tests/osbuild-composer/shared_lib.sh
source /usr/libexec/tests/osbuild-composer/api/common/executor.sh

#
# Supported image type to handler mapping
#
IMAGE_TYPE_GUEST="guest-image"

if (( $# != 1 )); then
    echo "Usage: $0 <image-type>"
    echo "Supported image types: ${IMAGE_TYPE_GUEST}"
    exit 1
fi

IMAGE_TYPE="$1"

# Select handler based on image type
case "${IMAGE_TYPE}" in
    "$IMAGE_TYPE_GUEST")
        source /usr/libexec/tests/osbuild-composer/api/bootc/guest.s3.sh
        ;;
    *)
        echo "Unknown image type: ${IMAGE_TYPE}"
        exit 1
        ;;
esac

ARTIFACTS="${ARTIFACTS:-/tmp/artifacts}"

# Container image used for cloud provider CLI tools
CONTAINER_IMAGE_CLOUD_TOOLS="quay.io/osbuild/cloud-tools:latest"

# Check available container runtime
if type -p podman 2>/dev/null >&2; then
    CONTAINER_RUNTIME=podman
elif type -p docker 2>/dev/null >&2; then
    CONTAINER_RUNTIME=docker
else
    echo "No container runtime found, install podman or docker."
    exit 2
fi

#
# Resolve bootc container ref from Schutzfile
#
ARCH=$(uname -m)
BOOTC_CONTAINER_REF="${BOOTC_CONTAINER_REF_OVERRIDE:-$(jq -r \
    ".[\"${ID}-${VERSION_ID}\"].dependencies.bootc[\"${IMAGE_TYPE}\"][\"${ARCH}\"].base" \
    Schutzfile)}"

if [ -z "$BOOTC_CONTAINER_REF" ] || [ "$BOOTC_CONTAINER_REF" = "null" ]; then
    echo "No bootc container ref found in Schutzfile for ${ID}-${VERSION_ID} / ${IMAGE_TYPE} / ${ARCH}"
    exit 1
fi
greenprint "Using bootc container ref: ${BOOTC_CONTAINER_REF}"

# Extract registry host from the container ref (everything before the first /)
REGISTRY_HOST="${BOOTC_CONTAINER_REF%%/*}"

#
# Verify environment
#
greenprint "Verifying environment"
checkEnv

#
# Provision the software under test (JWT mode).
# This sets up certificates, JWT composer/worker configs, mock auth servers,
# and starts Cloud API + remote worker units.
#
greenprint "Provisioning with JWT authentication"
/usr/libexec/osbuild-composer-test/provision.sh jwt

#
# Set up the database queue
#
DB_CONTAINER_NAME="osbuild-composer-db"
sudo "${CONTAINER_RUNTIME}" run -d --name "${DB_CONTAINER_NAME}" \
    --health-cmd "pg_isready -U postgres -d osbuildcomposer" --health-interval 2s \
    --health-timeout 2s --health-retries 10 \
    -e POSTGRES_USER=postgres \
    -e POSTGRES_PASSWORD=foobar \
    -e POSTGRES_DB=osbuildcomposer \
    -p 5432:5432 \
    --net host \
    quay.io/osbuild/postgres:13-alpine

sudo "${CONTAINER_RUNTIME}" logs "${DB_CONTAINER_NAME}"

pushd "$(mktemp -d)"
sudo dnf install -y go
go mod init temp
go install github.com/jackc/tern@latest
PGUSER=postgres PGPASSWORD=foobar PGDATABASE=osbuildcomposer PGHOST=localhost PGPORT=5432 \
    "$(go env GOPATH)"/bin/tern migrate -m /usr/share/tests/osbuild-composer/schemas
popd

#
# Cleanup handler
#
WORKDIR=$(mktemp -d)
KILL_PIDS=()

function dump_db() {
    sudo "${CONTAINER_RUNTIME}" exec "${DB_CONTAINER_NAME}" \
        psql -U postgres -d osbuildcomposer -c "SELECT type, args, result FROM jobs" \
        | sudo tee "${ARTIFACTS}/build-result.txt" > /dev/null
}

function cleanups() {
    greenprint "Cleaning up"
    set +eu

    # handler cleanup (e.g., remove S3 objects)
    cleanup

    dump_db

    sudo "${CONTAINER_RUNTIME}" kill "${DB_CONTAINER_NAME}"
    sudo "${CONTAINER_RUNTIME}" rm "${DB_CONTAINER_NAME}"

    cleanupExecutorKeypair

    sudo rm -rf "$WORKDIR"

    for P in "${KILL_PIDS[@]}"; do
        sudo pkill -P "$P"
    done
    set -eu
}
trap cleanups EXIT

#
# Configure composer with JWT + DB + bootc remote container sources.
# Appends DB and bootc settings to the JWT config already written by provision.sh.
#
greenprint "Configuring osbuild-composer"
sudo tee -a /etc/osbuild-composer/osbuild-composer.toml > /dev/null <<EOF

[worker]
pg_host = "localhost"
pg_port = "5432"
pg_database = "osbuildcomposer"
pg_user = "postgres"
pg_password = "foobar"
pg_ssl_mode = "disable"
pg_max_conns = 10

[bootc]
use_remote_container_source = true
EOF

sudo systemctl restart osbuild-composer

#
# Configure container registry authentication
#
greenprint "Configuring container registry authentication"
CONTAINER_AUTH_FILE="/etc/osbuild-worker/containerauth.json"
sudo "${CONTAINER_RUNTIME}" login \
    --authfile "${CONTAINER_AUTH_FILE}" \
    --username "${BOOTC_FOUNDRY_DERIVED_CONTAINERS_REGISTRY_USER}" \
    --password "${BOOTC_FOUNDRY_DERIVED_CONTAINERS_REGISTRY_PASS}" \
    "${REGISTRY_HOST}"

# Set REGISTRY_AUTH_FILE for all worker operations via systemd drop-in
WORKER_UNIT=$(sudo systemctl list-units | grep -o -E "osbuild-remote-worker@\S+\.service")
WORKER_DROPIN_DIR="/etc/systemd/system/${WORKER_UNIT}.d"
sudo mkdir -p "${WORKER_DROPIN_DIR}"
sudo tee "${WORKER_DROPIN_DIR}/registry-auth.conf" > /dev/null <<EOF
[Service]
Environment="REGISTRY_AUTH_FILE=${CONTAINER_AUTH_FILE}"
EOF

#
# Configure worker: executor + JWT auth
#
greenprint "Configuring osbuild-worker"

# Set up executor keypair (from executor.sh helper)
setupExecutorAWSClient
setupExecutorKeypair

# Append executor and AWS credentials config to the worker config
# (provision.sh already wrote [authentication] section)
sudo tee -a /etc/osbuild-worker/osbuild-worker.toml > /dev/null <<EOF

[osbuild_executor]
type = "aws.ec2"
key_name = "key-for-${INSTANCE_ID}-executor"
EOF

# Add AWS credentials if available
V2_AWS_ACCESS_KEY_ID="${V2_AWS_ACCESS_KEY_ID:-}"
V2_AWS_SECRET_ACCESS_KEY="${V2_AWS_SECRET_ACCESS_KEY:-}"
if [[ -n "$V2_AWS_ACCESS_KEY_ID" && -n "$V2_AWS_SECRET_ACCESS_KEY" ]]; then
    set +x
    sudo tee /etc/osbuild-worker/aws-credentials.toml > /dev/null <<EOF
[default]
aws_access_key_id = "$V2_AWS_ACCESS_KEY_ID"
aws_secret_access_key = "$V2_AWS_SECRET_ACCESS_KEY"
EOF
    sudo tee -a /etc/osbuild-worker/osbuild-worker.toml > /dev/null <<EOF

[aws]
credentials = "/etc/osbuild-worker/aws-credentials.toml"
bucket = "${AWS_BUCKET}"
EOF
    set -x
fi

sudo systemctl daemon-reload
sudo systemctl restart "${WORKER_UNIT}"

#
# Install cloud provider client tools (from handler)
#
greenprint "Installing cloud provider client tools"
installClient

#
# Wait for executor to come up and provision it
#
greenprint "Setting up executor"
waitForExecutorInstance
provisionExecutor
startExecutor

# Get worker unit for journal tailing
sudo journalctl -af -n 1 -u "${WORKER_UNIT}" &
KILL_PIDS+=("$!")

#
# Verify openapi endpoint is accessible
#
greenprint "Verifying Cloud API is accessible"
TOKEN=$(curl --request POST \
    --data "grant_type=refresh_token" \
    --data "refresh_token=$(cat /etc/osbuild-worker/token)" \
    --header "Content-Type: application/x-www-form-urlencoded" \
    --silent \
    --show-error \
    --fail \
    localhost:8081/token | jq -r .access_token)

curl \
    --silent \
    --show-error \
    --header "Authorization: Bearer ${TOKEN}" \
    http://localhost:443/api/image-builder-composer/v2/openapi | jq .

#
# Pre-compose verification: container ref should NOT be in local storage
#
greenprint "Pre-compose: verifying container ref is NOT in local storage"
if sudo podman image exists "${BOOTC_CONTAINER_REF}" 2>/dev/null; then
    redprint "Container ref ${BOOTC_CONTAINER_REF} already exists in local storage!"
    exit 1
fi
greenprint "Confirmed: container ref not in local storage"

#
# Compose execution
#
REQUEST_FILE="${WORKDIR}/compose_request.json"
export REQUEST_FILE WORKDIR ARCH IMAGE_TYPE BOOTC_CONTAINER_REF

greenprint "Creating compose request"
createReqFile

function sendCompose() {
    OUTPUT=$(mktemp)
    HTTPSTATUS=$(curl \
        --silent \
        --show-error \
        --header "Authorization: Bearer ${TOKEN}" \
        --header 'Content-Type: application/json' \
        --request POST \
        --data @"$1" \
        --write-out '%{http_code}' \
        --output "$OUTPUT" \
        http://localhost:443/api/image-builder-composer/v2/compose)

    if [ "$HTTPSTATUS" != "201" ]; then
        redprint "Sending compose request failed:"
        cat "$OUTPUT"
    fi

    test "$HTTPSTATUS" = "201"

    COMPOSE_ID=$(jq -r '.id' "$OUTPUT")
}

function waitForState() {
    local DESIRED_STATE="success"

    while true
    do
        OUTPUT=$(curl \
            --silent \
            --show-error \
            --header "Authorization: Bearer ${TOKEN}" \
            http://localhost:443/api/image-builder-composer/v2/composes/"$COMPOSE_ID")

        COMPOSE_STATUS=$(echo "$OUTPUT" | jq -r '.image_status.status')
        UPLOAD_STATUS=$(echo "$OUTPUT" | jq -r '.image_status.upload_status.status')
        UPLOAD_OPTIONS=$(echo "$OUTPUT" | jq -r '.image_status.upload_status.options')

        case "$COMPOSE_STATUS" in
            "$DESIRED_STATE")
                break
                ;;
            "pending"|"building"|"uploading"|"registering")
                ;;
            "failure")
                echo "Image compose failed"
                echo "API output: $OUTPUT"
                dump_db
                exit 1
                ;;
            *)
                echo "API returned unexpected image_status.status value: '$COMPOSE_STATUS'"
                echo "API output: $OUTPUT"
                dump_db
                exit 1
                ;;
        esac

        sleep 30
    done

    export UPLOAD_OPTIONS
}

greenprint "Sending bootc compose"
sendCompose "$REQUEST_FILE"

greenprint "Waiting for compose ${COMPOSE_ID} to finish"
waitForState

test "$UPLOAD_STATUS" = "success"

#
# Post-compose verification: container ref should now be in local storage
#
greenprint "Post-compose: verifying container ref IS in local storage"
if ! sudo podman image exists "${BOOTC_CONTAINER_REF}" 2>/dev/null; then
    redprint "Container ref ${BOOTC_CONTAINER_REF} was NOT pulled to local storage by BootcInfoResolveJob!"
    exit 1
fi
greenprint "Confirmed: container ref is in local storage"

#
# Verify upload status options (from handler)
#
greenprint "Checking upload status options"
checkUploadStatusOptions

#
# Verify the built image (from handler)
#
greenprint "Verifying built image"
verify

greenprint "DONE"
exit 0
```

- [ ] **Step 2: Make the file executable**

Run: `chmod +x test/cases/api-bootc-service.sh`

- [ ] **Step 3: Verify syntax and lint**

Run:
```bash
bash -n test/cases/api-bootc-service.sh
shellcheck test/cases/api-bootc-service.sh
```
Expected: No errors. Fix any shellcheck warnings before proceeding.

- [ ] **Step 4: Commit**

```bash
git add test/cases/api-bootc-service.sh
git commit -m "test: add bootc service e2e test driver

Cover the production-like bootc compose pipeline that
no existing test exercises: JWT + private registry +
executor + cloud upload, all in one flow."
```

---

### Task 5: Add CI Job

Add the GitLab CI job for running the new test.

**Files:**
- Modify: `.gitlab-ci.yml`

- [ ] **Step 1: Add CI job after the existing WorkerExecutor jobs**

Find the `WorkerExecutorFailure` job block (around line 890-901) and add the new job after it. Place it before the `CleanStore` job:

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

- [ ] **Step 2: Commit**

```bash
git add .gitlab-ci.yml
git commit -m "ci: wire up the bootc service e2e test

Run the new api-bootc-service.sh in CI so we catch
regressions in the production bootc compose flow on
every PR and merge."
```

---

### Task 6: Review and Verify

Final check that all files are consistent, syntactically valid, and the spec is fully covered.

- [ ] **Step 1: Verify all new bash files pass syntax and lint checks**

Run:
```bash
bash -n test/cases/api/common/executor.sh && echo "executor.sh syntax OK"
bash -n test/cases/api/bootc/guest.s3.sh && echo "guest.s3.sh syntax OK"
bash -n test/cases/api-bootc-service.sh && echo "api-bootc-service.sh syntax OK"
shellcheck test/cases/api/common/executor.sh && echo "executor.sh lint OK"
shellcheck test/cases/api/bootc/guest.s3.sh && echo "guest.s3.sh lint OK"
shellcheck test/cases/api-bootc-service.sh && echo "api-bootc-service.sh lint OK"
```

Expected: All six report OK.

- [ ] **Step 2: Verify Schutzfile is valid JSON**

Run: `python3 -m json.tool Schutzfile > /dev/null && echo "Schutzfile OK"`
Expected: `Schutzfile OK`

- [ ] **Step 3: Verify spec coverage**

Check against spec sections:
- File structure: `api-bootc-service.sh`, `api/bootc/guest.s3.sh`, `api/common/executor.sh` -- all created
- Schutzfile mapping: `rhel-10.1` bootc refs added with image-type -> arch -> ref-type structure
- JWT auth: Uses `provision.sh jwt` + appended config
- Worker config: Executor, registry auth (systemd drop-in), AWS credentials
- Executor provisioning: Via `executor.sh` shared helper
- Pre-compose container storage check: `podman image exists` (expect failure)
- Compose execution: `sendCompose` + `waitForState` with JWT bearer token
- Post-compose container storage check: `podman image exists` (expect success)
- Upload verification: `checkUploadStatusOptions` + `verify` from handler
- CI job: `API-bootc-service` with correct runner and IAM profile

- [ ] **Step 4: Verify no spec requirements were missed**

Review the spec's "Key Design Decisions" section against implementation:
1. Standalone test file -- yes, `api-bootc-service.sh`
2. Handler pattern -- yes, `case` statement in driver, handlers in `api/bootc/`
3. Handler reuse via sourcing -- yes, `guest.s3.sh` sources `api/aws.s3.sh`
4. REGISTRY_AUTH_FILE via systemd drop-in -- yes, no `[containers]` in worker TOML
5. JWT from start -- yes, `provision.sh jwt` called first
6. Schutzfile for container refs -- yes, full refs with override support
7. Executor via SSH -- yes, via `executor.sh` helper
8. Offline verification -- yes, inherits `verifyDisk()` from `api/common/s3.sh`
9. JWT setup extracted -- handled by existing `provision.sh jwt` + `run-mock-auth-servers.sh`
10. Executor setup extracted -- yes, `api/common/executor.sh`
