package v2_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/osbuild/images/pkg/bib/osinfo"
	v2 "github.com/osbuild/osbuild-composer/internal/cloudapi/v2"
	"github.com/osbuild/osbuild-composer/internal/common"
	"github.com/osbuild/osbuild-composer/internal/jobqueue/fsjobqueue"
	"github.com/osbuild/osbuild-composer/internal/worker"
	"github.com/osbuild/osbuild-composer/internal/worker/clienterrors"
)

// newTestWorkerServer creates a minimal worker server for testing
// handleBootcPreManifest directly, without the full v2 server stack.
func newTestWorkerServer(t *testing.T) *worker.Server {
	t.Helper()
	jobsDir := filepath.Join(t.TempDir(), "jobs")
	err := os.Mkdir(jobsDir, 0755)
	require.NoError(t, err)

	q, err := fsjobqueue.New(jobsDir)
	require.NoError(t, err)

	return worker.NewServer(nil, q, worker.Config{
		BasePath: "/api/worker/v1",
	})
}

// rawValidBaseBootcInfoResult returns a marshaled BootcInfoResolveJobResult
// matching the test container used across bootc pre-manifest handler tests.
func rawValidBaseBootcInfoResult(t *testing.T) json.RawMessage {
	t.Helper()
	osInfo := &osinfo.Info{
		OSRelease: osinfo.OSRelease{
			ID:        "centos",
			VersionID: "9",
		},
	}
	data, err := json.Marshal(osInfo)
	require.NoError(t, err)

	baseResult := worker.BootcInfoResolveJobResult{
		Info: &worker.BootcContainerInfo{
			Imgref:        "quay.io/centos-bootc/centos-bootc:stream9",
			ImageID:       "sha256:abc123",
			Arch:          "x86_64",
			DefaultRootFs: "xfs",
			Size:          1073741824,
			OSInfo:        data,
		},
	}
	b, err := json.Marshal(baseResult)
	require.NoError(t, err)

	return b
}

func TestHandleBootcPreManifest_Errors(t *testing.T) {
	tests := []struct {
		name               string
		job                *worker.BootcPreManifestJob
		staticArgsOverride func(t *testing.T) json.RawMessage
		dynArgs            func(t *testing.T) []json.RawMessage
		wantErrID          clienterrors.ClientErrorCode
		wantReasonContains string
	}{
		{
			name: "invalid_static_args_JSON",
			job: &worker.BootcPreManifestJob{
				ImageType:                  "qcow2",
				Seed:                       42,
				BootcBaseResolveDynArgsIdx: common.ToPtr(0),
			},
			staticArgsOverride: func(t *testing.T) json.RawMessage {
				t.Helper()
				return json.RawMessage(`{invalid`)
			},
			wantErrID:          clienterrors.ErrorParsingJobArgs,
			wantReasonContains: "Error parsing bootc pre-manifest job args",
		},
		{
			name: "missing_base_index",
			job: &worker.BootcPreManifestJob{
				ImageType:                  "qcow2",
				Seed:                       42,
				BootcBaseResolveDynArgsIdx: nil,
			},
			wantErrID:          clienterrors.ErrorParsingDynamicArgs,
			wantReasonContains: "BootcBaseResolveDynArgsIdx is missing or out of range",
		},
		{
			name: "base_index_out_of_range",
			job: &worker.BootcPreManifestJob{
				ImageType:                  "qcow2",
				Seed:                       42,
				BootcBaseResolveDynArgsIdx: common.ToPtr(5),
			},
			wantErrID:          clienterrors.ErrorParsingDynamicArgs,
			wantReasonContains: "BootcBaseResolveDynArgsIdx is missing or out of range",
		},
		{
			name: "invalid_base_dynArg_JSON",
			job: &worker.BootcPreManifestJob{
				ImageType:                  "qcow2",
				Seed:                       42,
				BootcBaseResolveDynArgsIdx: common.ToPtr(0),
			},
			dynArgs: func(t *testing.T) []json.RawMessage {
				t.Helper()
				return []json.RawMessage{json.RawMessage(`{invalid json`)}
			},
			wantErrID:          clienterrors.ErrorParsingDynamicArgs,
			wantReasonContains: "Error parsing base bootc info resolve result",
		},
		{
			name: "base_dependency_failed",
			job: &worker.BootcPreManifestJob{
				ImageType:                  "qcow2",
				Seed:                       42,
				BootcBaseResolveDynArgsIdx: common.ToPtr(0),
			},
			dynArgs: func(t *testing.T) []json.RawMessage {
				t.Helper()
				failedResult := worker.BootcInfoResolveJobResult{
					JobResult: worker.JobResult{
						JobError: clienterrors.New(
							clienterrors.ErrorBootcInfoResolve,
							"container not found", nil,
						),
					},
				}
				b, err := json.Marshal(failedResult)
				require.NoError(t, err)
				return []json.RawMessage{b}
			},
			wantErrID:          clienterrors.ErrorJobDependency,
			wantReasonContains: "Base bootc info resolve dependency failed",
		},
		{
			name: "build_index_out_of_range",
			job: &worker.BootcPreManifestJob{
				ImageType:                   "qcow2",
				Seed:                        42,
				BootcBaseResolveDynArgsIdx:  common.ToPtr(0),
				BootcBuildResolveDynArgsIdx: common.ToPtr(5),
			},
			dynArgs: func(t *testing.T) []json.RawMessage {
				t.Helper()
				return []json.RawMessage{rawValidBaseBootcInfoResult(t)}
			},
			wantErrID:          clienterrors.ErrorParsingDynamicArgs,
			wantReasonContains: "BootcBuildResolveDynArgsIdx is out of range",
		},
		{
			name: "build_dependency_failed",
			job: &worker.BootcPreManifestJob{
				ImageType:                   "qcow2",
				Seed:                        42,
				BootcBaseResolveDynArgsIdx:  common.ToPtr(0),
				BootcBuildResolveDynArgsIdx: common.ToPtr(1),
			},
			dynArgs: func(t *testing.T) []json.RawMessage {
				t.Helper()
				failedBuildResult := worker.BootcInfoResolveJobResult{
					JobResult: worker.JobResult{
						JobError: clienterrors.New(
							clienterrors.ErrorBootcInfoResolve,
							"build container not found", nil,
						),
					},
				}
				buildJSON, err := json.Marshal(failedBuildResult)
				require.NoError(t, err)
				return []json.RawMessage{rawValidBaseBootcInfoResult(t), buildJSON}
			},
			wantErrID:          clienterrors.ErrorJobDependency,
			wantReasonContains: "Build bootc info resolve dependency failed",
		},
		{
			name: "invalid_image_type",
			job: &worker.BootcPreManifestJob{
				ImageType:                  "nonexistent-image-type",
				Seed:                       42,
				BootcBaseResolveDynArgsIdx: common.ToPtr(0),
			},
			dynArgs: func(t *testing.T) []json.RawMessage {
				t.Helper()
				return []json.RawMessage{rawValidBaseBootcInfoResult(t)}
			},
			wantErrID:          clienterrors.ErrorManifestGeneration,
			wantReasonContains: "invalid image type: nonexistent-image-type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := newTestWorkerServer(t)
			preManifestJobID, err := ws.EnqueueBootcPreManifestJob(tt.job, nil, "")
			require.NoError(t, err)
			jobID, token, _, _, _, err := ws.RequestJob(
				context.Background(), "",
				[]string{worker.JobTypeBootcPreManifest}, []string{""}, uuid.Nil,
			)
			require.NoError(t, err)

			var staticArgs json.RawMessage
			if tt.staticArgsOverride != nil {
				staticArgs = tt.staticArgsOverride(t)
			} else {
				var err error
				staticArgs, err = json.Marshal(tt.job)
				require.NoError(t, err)
			}

			var dynArgs []json.RawMessage
			if tt.dynArgs != nil {
				dynArgs = tt.dynArgs(t)
			}

			v2.HandleBootcPreManifest(ws, jobID, token, staticArgs, dynArgs)

			var readResult worker.BootcPreManifestJobResult
			jobInfo, err := ws.BootcPreManifestJobInfo(preManifestJobID, &readResult)
			require.NoError(t, err)
			require.NotNil(t, jobInfo)
			assert.False(t, jobInfo.JobStatus.Finished.IsZero(), "job should be finished (defer always calls FinishJob)")
			require.NotNil(t, readResult.JobError)
			assert.Equal(t, tt.wantErrID, readResult.JobError.ID)
			assert.Contains(t, readResult.JobError.Reason, tt.wantReasonContains)
		})
	}
}

// enqueuePreManifestWithResolvedDep enqueues a bootc info-resolve job,
// finishes it with a valid result, and enqueues a pre-manifest job that
// depends on it. Returns the pre-manifest job ID.
func enqueuePreManifestWithResolvedDep(t *testing.T, ws *worker.Server) uuid.UUID {
	t.Helper()
	infoResolveJob := &worker.BootcInfoResolveJob{
		Ref:         "quay.io/centos-bootc/centos-bootc:stream9",
		Arch:        "x86_64",
		FullResolve: true,
	}
	infoResolveJobID, err := ws.EnqueueBootcInfoResolveJob("x86_64", infoResolveJob, "")
	require.NoError(t, err)

	preManifestJob := &worker.BootcPreManifestJob{
		ImageType:                  "qcow2",
		Seed:                       42,
		BootcBaseResolveDynArgsIdx: common.ToPtr(0),
	}
	preManifestJobID, err := ws.EnqueueBootcPreManifestJob(
		preManifestJob, []uuid.UUID{infoResolveJobID}, "",
	)
	require.NoError(t, err)

	_, infoToken, _, _, _, err := ws.RequestJob(
		context.Background(), "x86_64",
		[]string{worker.JobTypeBootcInfoResolve}, []string{""}, uuid.Nil,
	)
	require.NoError(t, err)

	err = ws.FinishJob(infoToken, rawValidBaseBootcInfoResult(t))
	require.NoError(t, err)

	return preManifestJobID
}

// assertValidPreManifestResult checks that a BootcPreManifestJobResult
// completed without error and contains the expected container resolve data
// for the test fixture container (centos-bootc:stream9 on x86_64).
func assertValidPreManifestResult(t *testing.T, result worker.BootcPreManifestJobResult) {
	t.Helper()

	require.Nil(t, result.JobError, "expected no job error, got: %v", result.JobError)

	assert.Equal(t, "x86_64", result.ContainerResolveJobArgs.Arch)
	assert.NotEmpty(t, result.ContainerResolveJobArgs.Specs, "expected at least one container spec")

	foundBaseRef := false
	for _, spec := range result.ContainerResolveJobArgs.Specs {
		if spec.Source == "quay.io/centos-bootc/centos-bootc:stream9" {
			foundBaseRef = true
			break
		}
	}
	assert.True(t, foundBaseRef, "expected container spec with source quay.io/centos-bootc/centos-bootc:stream9")
}

// TestHandleBootcPreManifest_HappyPath tests the happy path for the
// BootcPreManifest job: enqueue a pre-manifest job with a completed
// dependency, and verify the job finishes successfully. Without going
// through the loop.
func TestHandleBootcPreManifest_HappyPath(t *testing.T) {
	workerServer := newTestWorkerServer(t)
	preManifestJobID := enqueuePreManifestWithResolvedDep(t, workerServer)

	// Dequeue the pre-manifest job (it should be pending now)
	jobID, preManifestToken, _, staticArgs, dynArgs, err := workerServer.RequestJob(
		context.Background(), "",
		[]string{worker.JobTypeBootcPreManifest}, []string{""}, uuid.Nil,
	)
	require.NoError(t, err)

	// Call the handler
	v2.HandleBootcPreManifest(workerServer, jobID, preManifestToken, staticArgs, dynArgs)

	// Verify the job finished successfully
	var readResult worker.BootcPreManifestJobResult
	jobInfo, err := workerServer.BootcPreManifestJobInfo(preManifestJobID, &readResult)
	require.NoError(t, err)
	require.NotNil(t, jobInfo)
	assert.False(t, jobInfo.JobStatus.Finished.IsZero(), "job should be finished")

	assertValidPreManifestResult(t, readResult)
}

// TestBootcPreManifestLoop_PicksUpJob tests the full loop lifecycle:
// Start v2 server (which starts the loop), enqueue a pre-manifest
// job with completed dependencies, and verify the job gets finished.
func TestBootcPreManifestLoop_PicksUpJob(t *testing.T) {
	workerServer := newTestWorkerServer(t)

	// Create v2 server which starts the bootcPreManifestLoop
	v2Server := v2.NewServer(workerServer, nil, nil, v2.ServerConfig{})
	require.NotNil(t, v2Server)
	t.Cleanup(v2Server.Shutdown)

	preManifestJobID := enqueuePreManifestWithResolvedDep(t, workerServer)

	// Wait for the loop to pick up and finish the pre-manifest job.
	// Poll with timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			require.FailNow(t, "timed out waiting for bootcPreManifestLoop to finish job")
		default:
		}

		var readResult worker.BootcPreManifestJobResult
		jobInfo, err := workerServer.BootcPreManifestJobInfo(preManifestJobID, &readResult)
		if err == nil && !jobInfo.JobStatus.Finished.IsZero() {
			assertValidPreManifestResult(t, readResult)
			return
		}

		// Small sleep to avoid busy-waiting
		time.Sleep(50 * time.Millisecond)
	}
}
