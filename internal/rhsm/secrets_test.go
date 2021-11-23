package rhsm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var VALID_REPO = `[jws]
name = Red Hat JBoss Web Server
baseurl = https://cdn.redhat.com/content/dist/middleware/jws/1.0/$basearch/os
enabled = 0
gpgcheck = 1
gpgkey = file://
sslverify = 1
sslcacert = /etc/rhsm/ca/redhat-uep.pem
sslclientkey = /etc/pki/entitlement/123-key.pem
sslclientcert = /etc/pki/entitlement/456.pem
metadata_expire = 86400
enabled_metadata = 0

[rhel-atomic]
name = Red Hat Container Development Kit
baseurl = https://cdn.redhat.com/content/dist/rhel/atomic/7/7Server/$basearch/os
enabled = 0
gpgcheck = 1
gpgkey = http://
sslverify = 1
sslcacert = /etc/rhsm/ca/redhat-uep.pem
sslclientkey = /etc/pki/entitlement/789-key.pem
sslclientcert = /etc/pki/entitlement/101112.pem
metadata_expire = 86400
enabled_metadata = 0

[rhel-8-for-x86_64-appstream-rpms]
name = Red Hat Enterprise Linux 8 for x86_64 - AppStream (RPMs)
baseurl = https://cdn.redhat.com/content/dist/rhel8/$releasever/x86_64/appstream/os
enabled = 1
gpgcheck = 1
gpgkey = file:///etc/pki/rpm-gpg/RPM-GPG-KEY-redhat-release
sslverify = 1
sslcacert = /etc/rhsm/ca/redhat-uep.pem
sslclientkey = /etc/pki/entitlement/121-key.pem
sslclientcert = /etc/pki/entitlement/121.pem
metadata_expire = 86400
enabled_metadata = 1

[rhel-8-for-x86_64-baseos-rpms]
name = Red Hat Enterprise Linux 8 for x86_64 - BaseOS (RPMs)
baseurl = https://cdn.redhat.com/content/dist/rhel8/$releasever/x86_64/baseos/os
enabled = 1
gpgcheck = 1
gpgkey = file:///etc/pki/rpm-gpg/RPM-GPG-KEY-redhat-release
sslverify = 1
sslcacert = /etc/rhsm/ca/redhat-uep.pem
sslclientkey = /etc/pki/entitlement/121-key.pem
sslclientcert = /etc/pki/entitlement/121.pem
metadata_expire = 86400
enabled_metadata = 1
`

func TestParseRepoFile(t *testing.T) {
	input := []byte(VALID_REPO)
	repoFileContent, err := parseRepoFile(input)
	require.NoError(t, err, "Failed to parse the .repo file")
	subscriptions := Subscriptions{
		available: repoFileContent,
	}
	secrets, err := subscriptions.GetSecretsForBaseurl("https://cdn.redhat.com/content/dist/middleware/jws/1.0/x86_64/os", "x86_64", "")
	require.NoError(t, err, "Failed to get secrets for a baseurl")
	assert.Equal(t, secrets.SSLCACert, "/etc/rhsm/ca/redhat-uep.pem", "Unexpected path to the CA certificate")
	assert.Equal(t, secrets.SSLClientCert, "/etc/pki/entitlement/456.pem", "Unexpected path to the client cert")
	assert.Equal(t, secrets.SSLClientKey, "/etc/pki/entitlement/123-key.pem", "Unexpected path to the client key")

	// BaseURL from `repositories/rhel-85.json`
	x86ComposerRepoBaseURL := "https://cdn.redhat.com/content/dist/rhel8/8.5/x86_64/baseos/os"
	x86ArchName := "x86_64"
	// value returned by rhel_85 Distro.Releasever()
	rhel85distroReleasever := "8"

	secrets, err = subscriptions.GetSecretsForBaseurl(x86ComposerRepoBaseURL, x86ArchName, rhel85distroReleasever)
	require.NoError(t, err, "Failed to get secrets for a baseurl")
	assert.Equal(t, secrets.SSLCACert, "/etc/rhsm/ca/redhat-uep.pem", "Unexpected path to the CA certificate")
	assert.Equal(t, secrets.SSLClientCert, "/etc/pki/entitlement/121.pem", "Unexpected path to the client cert")
	assert.Equal(t, secrets.SSLClientKey, "/etc/pki/entitlement/121-key.pem", "Unexpected path to the client key")
}
