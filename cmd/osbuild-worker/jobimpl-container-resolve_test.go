package main_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	main "github.com/osbuild/osbuild-composer/cmd/osbuild-worker"
	"github.com/osbuild/osbuild-composer/internal/common"
	"github.com/osbuild/osbuild-composer/internal/worker"
	"github.com/osbuild/osbuild-composer/internal/worker/clienterrors"
)

func unmarshalResolveResult(t *testing.T, raw json.RawMessage) worker.ContainerResolveJobResult {
	t.Helper()
	var r worker.ContainerResolveJobResult
	require.NoError(t, json.Unmarshal(raw, &r))
	return r
}

func TestContainerResolveJobRun(t *testing.T) {
	assertNoopResolveResult := func(t *testing.T, raw json.RawMessage) {
		r := unmarshalResolveResult(t, raw)
		assert.Nil(t, r.JobError)
		assert.Empty(t, r.Specs)
	}

	tests := []struct {
		name               string
		jobArgs            *worker.ContainerResolveJob
		jobArgsRaw         json.RawMessage // if jobArgs is nil, use this instead
		finishErr          error
		wantRunErr         bool
		wantErrSubstr      string
		wantFinishCalled   bool
		verifyFinishResult func(t *testing.T, raw json.RawMessage)
	}{
		{
			name: "empty specs - no-op",
			jobArgs: &worker.ContainerResolveJob{
				Arch:  "x86_64",
				Specs: []worker.ContainerSpec{},
			},
			wantFinishCalled:   true,
			verifyFinishResult: assertNoopResolveResult,
		},
		{
			name: "nil specs - no-op",
			jobArgs: &worker.ContainerResolveJob{
				Arch:  "x86_64",
				Specs: nil,
			},
			wantFinishCalled:   true,
			verifyFinishResult: assertNoopResolveResult,
		},
		{
			name:             "args unmarshal error",
			jobArgsRaw:       json.RawMessage(`{invalid json`),
			wantRunErr:       true,
			wantFinishCalled: true,
		},
		{
			name: "finish error is logged not returned",
			jobArgs: &worker.ContainerResolveJob{
				Arch:  "x86_64",
				Specs: []worker.ContainerSpec{},
			},
			finishErr:        fmt.Errorf("connection lost"),
			wantRunErr:       false,
			wantFinishCalled: true,
		},
		{
			name: "specs with unresolvable container",
			jobArgs: &worker.ContainerResolveJob{
				Arch: "x86_64",
				Specs: []worker.ContainerSpec{
					{
						Source: "localhost:1/nonexistent/image:latest",
						Name:   "test-container",
					},
				},
			},
			wantRunErr:       true,
			wantErrSubstr:    "Error resolving containers",
			wantFinishCalled: true,
			verifyFinishResult: func(t *testing.T, raw json.RawMessage) {
				r := unmarshalResolveResult(t, raw)
				assert.NotNil(t, r.JobError, "expected job error for unresolvable container")
				assert.Equal(t, clienterrors.ErrorContainerResolution, r.JobError.ID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rawArgs json.RawMessage
			if tt.jobArgs != nil {
				rawArgs = marshalJobArgs(t, *tt.jobArgs)
			} else {
				rawArgs = tt.jobArgsRaw
			}

			jobMock := newMockJob(t, worker.JobTypeContainerResolve, rawArgs)
			jobMock.finishErr = tt.finishErr

			impl := &main.ContainerResolveJobImpl{AuthFilePath: ""}
			runErr := impl.Run(jobMock)

			if tt.wantRunErr {
				require.Error(t, runErr)
				if tt.wantErrSubstr != "" {
					assert.Contains(t, runErr.Error(), tt.wantErrSubstr)
				}
			} else {
				require.NoError(t, runErr)
			}

			assert.Equal(t, tt.wantFinishCalled, jobMock.finishCalled, "Finish() called state")
			if tt.verifyFinishResult != nil && jobMock.finishCalled && tt.finishErr == nil {
				tt.verifyFinishResult(t, jobMock.finishResult)
			}
		})
	}
}

func TestContainerResolveJobRunDynArgs(t *testing.T) {
	tests := []struct {
		name               string
		jobArgs            worker.ContainerResolveJob
		dynArgs            []interface{}
		wantRunErr         bool
		wantErrSubstr      string
		wantFinishCalled   bool
		verifyFinishResult func(t *testing.T, raw json.RawMessage)
	}{
		{
			name: "dynArgs with PreManifestDynArgsIdx reads specs from BootcPreManifestJobResult",
			jobArgs: worker.ContainerResolveJob{
				Arch:                  "x86_64",
				Specs:                 nil,
				PreManifestDynArgsIdx: common.ToPtr(0),
			},
			dynArgs: []interface{}{
				worker.BootcPreManifestJobResult{
					ContainerResolveJobArgs: &worker.ContainerResolveJob{
						Arch: "x86_64",
						Specs: []worker.ContainerSpec{
							{
								Source: "localhost:1/nonexistent/image:latest",
								Name:   "test-from-dynargs",
							},
						},
					},
				},
			},
			wantRunErr:       true,
			wantErrSubstr:    "Error resolving containers",
			wantFinishCalled: true,
			verifyFinishResult: func(t *testing.T, raw json.RawMessage) {
				r := unmarshalResolveResult(t, raw)
				assert.NotNil(t, r.JobError, "expected job error for unresolvable container from dynArgs")
				assert.Equal(t, clienterrors.ErrorContainerResolution, r.JobError.ID, "expected ErrorContainerResolution, got %d", r.JobError.ID)
			},
		},
		{
			name: "dynArgs with empty ContainerResolveJobArgs is no-op",
			jobArgs: worker.ContainerResolveJob{
				Arch:                  "x86_64",
				Specs:                 nil,
				PreManifestDynArgsIdx: common.ToPtr(0),
			},
			dynArgs:          []interface{}{worker.BootcPreManifestJobResult{}},
			wantFinishCalled: true,
			verifyFinishResult: func(t *testing.T, raw json.RawMessage) {
				r := unmarshalResolveResult(t, raw)
				assert.Nil(t, r.JobError, "expected no job error for empty ContainerResolveJobArgs")
				assert.Empty(t, r.Specs, "expected empty result specs when ContainerResolveJobArgs is empty")
			},
		},
		{
			name: "dynArgs index out of range",
			jobArgs: worker.ContainerResolveJob{
				Arch:                  "x86_64",
				Specs:                 nil,
				PreManifestDynArgsIdx: common.ToPtr(5),
			},
			wantRunErr:       true,
			wantErrSubstr:    "Error reading container resolve args from dynamic args",
			wantFinishCalled: true,
			verifyFinishResult: func(t *testing.T, raw json.RawMessage) {
				r := unmarshalResolveResult(t, raw)
				require.NotNil(t, r.JobError, "expected job error for out-of-range dynArgs index")
				assert.Equal(t, clienterrors.ErrorParsingDynamicArgs, r.JobError.ID, "expected ErrorParsingDynamicArgs, got %d", r.JobError.ID)
			},
		},
		{
			name: "dynArgs dependency failed",
			jobArgs: worker.ContainerResolveJob{
				Arch:                  "x86_64",
				Specs:                 nil,
				PreManifestDynArgsIdx: common.ToPtr(0),
			},
			dynArgs: []interface{}{
				worker.BootcPreManifestJobResult{
					JobResult: worker.JobResult{
						JobError: clienterrors.New(clienterrors.ErrorBuildJob, "pre-manifest failed", nil),
					},
				},
			},
			wantRunErr:       true,
			wantErrSubstr:    "Error reading container resolve args from dynamic args",
			wantFinishCalled: true,
			verifyFinishResult: func(t *testing.T, raw json.RawMessage) {
				r := unmarshalResolveResult(t, raw)
				require.NotNil(t, r.JobError, "expected job error for failed dependency")
				assert.Equal(t, clienterrors.ErrorJobDependency, r.JobError.ID, "expected ErrorJobDependency, got %d", r.JobError.ID)
			},
		},
		{
			name: "no dynArgs without PreManifestDynArgsIdx - standard flow unchanged",
			jobArgs: worker.ContainerResolveJob{
				Arch: "x86_64",
				Specs: []worker.ContainerSpec{
					{
						Source: "localhost:1/nonexistent/image:latest",
						Name:   "test-container",
					},
				},
				PreManifestDynArgsIdx: nil,
			},
			wantRunErr:       true,
			wantErrSubstr:    "Error resolving containers",
			wantFinishCalled: true,
			verifyFinishResult: func(t *testing.T, raw json.RawMessage) {
				r := unmarshalResolveResult(t, raw)
				assert.NotNil(t, r.JobError, "expected job error for unresolvable container")
				assert.Equal(t, clienterrors.ErrorContainerResolution, r.JobError.ID, "expected ErrorContainerResolution, got %d", r.JobError.ID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jobMock := newMockJob(t, worker.JobTypeContainerResolve, marshalJobArgs(t, tt.jobArgs), tt.dynArgs...)

			impl := &main.ContainerResolveJobImpl{AuthFilePath: ""}
			runErr := impl.Run(jobMock)

			if tt.wantRunErr {
				require.Error(t, runErr)
				if tt.wantErrSubstr != "" {
					assert.Contains(t, runErr.Error(), tt.wantErrSubstr)
				}
			} else {
				require.NoError(t, runErr)
			}

			assert.Equal(t, tt.wantFinishCalled, jobMock.finishCalled, "Finish() called state")
			if tt.verifyFinishResult != nil && jobMock.finishCalled {
				tt.verifyFinishResult(t, jobMock.finishResult)
			}
		})
	}
}
