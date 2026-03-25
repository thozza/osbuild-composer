package main_test

import (
	"encoding/json"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/osbuild/images/pkg/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	main "github.com/osbuild/osbuild-composer/cmd/osbuild-worker"
	"github.com/osbuild/osbuild-composer/internal/worker"
)

// mockJob implements worker.Job for unit testing job implementations.
type mockJob struct {
	id          uuid.UUID
	jobType     string
	argsJSON    json.RawMessage
	dynArgsJSON []json.RawMessage
	result      interface{}
}

func (m *mockJob) Id() uuid.UUID                                              { return m.id }
func (m *mockJob) Type() string                                               { return m.jobType }
func (m *mockJob) Update(result interface{}) error                            { return nil }
func (m *mockJob) Canceled() (bool, error)                                    { return false, nil }
func (m *mockJob) UploadArtifact(name string, readSeeker io.ReadSeeker) error { return nil }

func (m *mockJob) Args(args interface{}) error {
	return json.Unmarshal(m.argsJSON, args)
}

func (m *mockJob) NDynamicArgs() int {
	return len(m.dynArgsJSON)
}

func (m *mockJob) DynamicArgs(i int, args interface{}) error {
	if i >= len(m.dynArgsJSON) {
		return nil
	}
	return json.Unmarshal(m.dynArgsJSON[i], args)
}

func (m *mockJob) Finish(result interface{}) error {
	m.result = result
	return nil
}

func newMockJob(t *testing.T, args interface{}) *mockJob {
	t.Helper()
	argsJSON, err := json.Marshal(args)
	require.NoError(t, err)
	return &mockJob{
		id:       uuid.New(),
		jobType:  worker.JobTypeContainerResolve,
		argsJSON: argsJSON,
	}
}

func TestContainerResolveStaticArgs(t *testing.T) {
	restore := main.MockResolveContainerSpecs(
		func(arch, authFilePath string, specs []worker.ContainerSpec) ([]container.Spec, error) {
			result := make([]container.Spec, len(specs))
			for i, s := range specs {
				result[i] = container.Spec{
					Source:  s.Source,
					Digest:  "sha256:abcdef1234567890",
					ImageID: "id-" + s.Name,
				}
			}
			return result, nil
		},
	)
	defer restore()

	tlsVerify := true
	job := newMockJob(t, worker.ContainerResolveJob{
		Arch: "x86_64",
		Specs: []worker.ContainerSpec{
			{Source: "registry.example.com/img:tag", Name: "test-container", TLSVerify: &tlsVerify},
		},
	})

	impl := &main.ContainerResolveJobImpl{}
	err := impl.Run(job)
	require.NoError(t, err)

	result, ok := job.result.(*worker.ContainerResolveJobResult)
	require.True(t, ok)
	assert.Nil(t, result.JobError)
	require.Len(t, result.Specs, 1)
	assert.Equal(t, "registry.example.com/img:tag", result.Specs[0].Source)
	assert.Equal(t, "sha256:abcdef1234567890", result.Specs[0].Digest)
}

func TestContainerResolveEmptySpecs(t *testing.T) {
	restore := main.MockResolveContainerSpecs(
		func(arch, authFilePath string, specs []worker.ContainerSpec) ([]container.Spec, error) {
			return []container.Spec{}, nil
		},
	)
	defer restore()

	job := newMockJob(t, worker.ContainerResolveJob{
		Arch:  "x86_64",
		Specs: []worker.ContainerSpec{},
	})

	impl := &main.ContainerResolveJobImpl{}
	err := impl.Run(job)
	require.NoError(t, err)

	result, ok := job.result.(*worker.ContainerResolveJobResult)
	require.True(t, ok)
	assert.Nil(t, result.JobError)
	assert.Empty(t, result.Specs)
}
