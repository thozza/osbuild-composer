package v2

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/osbuild/osbuild-composer/internal/worker"
)

type ManifestJobDependencies = manifestJobDependencies

// MockSerializeManifestFunc overrides the serializeManifestFunc for testing
func MockSerializeManifestFunc(f func(ctx context.Context, getManifestSource ManifestSourceFunc, workers *worker.Server, dependencies manifestJobDependencies, manifestJobID uuid.UUID, seed int64)) (restore func()) {
	originalSerializeManifestFunc := serializeManifestFunc
	serializeManifestFunc = f
	return func() {
		serializeManifestFunc = originalSerializeManifestFunc
	}
}

// HandleBootcPreManifest exports handleBootcPreManifest for testing.
var HandleBootcPreManifest = handleBootcPreManifest

// HandleBootcPreManifestParams bundles the parameters for handleBootcPreManifest
// to make test setup clearer.
type HandleBootcPreManifestParams struct {
	Workers    *worker.Server
	Token      uuid.UUID
	StaticArgs json.RawMessage
	DynArgs    []json.RawMessage
}
