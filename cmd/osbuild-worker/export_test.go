package main

import (
	"github.com/osbuild/images/pkg/container"

	"github.com/osbuild/osbuild-composer/internal/worker"
)

var (
	WorkerClientErrorFrom         = workerClientErrorFrom
	MakeJobErrorFromOsbuildOutput = makeJobErrorFromOsbuildOutput
	Main                          = main
	ParseManifestPipelines        = parseManifestPipelines
	ResolvePipelineNames          = resolvePipelineNames
)

func MockRun(new func()) (restore func()) {
	saved := run
	run = new
	return func() {
		run = saved
	}
}

// MockResolveContainerSpecs overrides resolveContainerSpecs for testing.
func MockResolveContainerSpecs(f func(arch, authFilePath string, specs []worker.ContainerSpec) ([]container.Spec, error)) (restore func()) {
	saved := resolveContainerSpecs
	resolveContainerSpecs = f
	return func() {
		resolveContainerSpecs = saved
	}
}
