#!/usr/bin/bash

# Handler for bootc image-installer compose with aws.s3 upload target.
# Sources the guest.s3.sh handler to inherit installClient(),
# checkUploadStatusOptions(), checkEnv(), and cleanup().
# Overrides createReqFile() and verify() for installer-specific behavior.

source /usr/libexec/tests/osbuild-composer/api/bootc/guest.s3.sh

# Override: download the ISO from S3 and verify it is a valid ISO image
function verify() {
    local S3_URL
    S3_URL=$(echo "$UPLOAD_OPTIONS" | jq -r '.url')
    greenprint "Verifying S3 object at ${S3_URL}"

    # Tag the resource as a test file
    local S3_FILENAME
    S3_FILENAME=$(echo "import urllib.parse; print(urllib.parse.urlsplit('$S3_URL').path.strip('/'))" | python3 -)

    $AWS_CMD s3api put-object-tagging \
        --bucket "${AWS_BUCKET}" \
        --key "${S3_FILENAME}" \
        --tagging '{"TagSet": [{ "Key": "gitlab-ci-test", "Value": "true" }]}'

    greenprint "Downloading installer ISO"
    curl --fail "${S3_URL}" --output "${WORKDIR}/installer.iso"

    greenprint "Verifying ISO image format"
    file "${WORKDIR}/installer.iso" | grep -q "ISO 9660"

    greenprint "✅ Successfully verified installer ISO"
}

# Override: create bootc installer compose request
function createReqFile() {
    cat > "$REQUEST_FILE" << EOF
{
  "bootc": {
    "reference": "$BOOTC_CONTAINER_REF",
    "installer_payload_ref": "$BOOTC_INSTALLER_PAYLOAD_REF"
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
