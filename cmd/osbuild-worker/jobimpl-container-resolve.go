package main

import (
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/osbuild/images/pkg/container"
	"github.com/osbuild/osbuild-composer/internal/worker"
	"github.com/osbuild/osbuild-composer/internal/worker/clienterrors"
)

type ContainerResolveJobImpl struct {
	AuthFilePath string
}

func (impl *ContainerResolveJobImpl) Run(job worker.Job) error {
	logWithId := logrus.WithField("jobId", job.Id())

	result := worker.ContainerResolveJobResult{}
	defer func() {
		if r := recover(); r != nil {
			logWithId.Errorf("Recovered from panic in ContainerResolveJobImpl.Run: %v", r)
			result.JobError = clienterrors.New(clienterrors.ErrorJobPanicked, "Error resolving containers", r)
		}

		err := job.Finish(&result)
		if err != nil {
			logWithId.Errorf("Error reporting job result: %v", err)
		}
	}()

	var args worker.ContainerResolveJob
	if err := job.Args(&args); err != nil {
		result.JobError = clienterrors.New(
			clienterrors.ErrorParsingJobArgs, "Error parsing container resolve job args: "+err.Error(), nil)
		return fmt.Errorf("Error parsing container resolve job args: %v", err)
	}

	// If static args have no specs and a dynArgs index is set, read args from the BootcPreManifest dependency result.
	if len(args.Specs) == 0 && args.PreManifestDynArgsIdx != nil {
		dynArgsResult, dynArgsErr := readContainerResolveArgsFromDynArgs(job, *args.PreManifestDynArgsIdx)
		if dynArgsErr != nil {
			result.JobError = dynArgsErr
			return fmt.Errorf("Error reading container resolve args from dynamic args: %v", dynArgsErr)
		}
		if dynArgsResult != nil {
			args = *dynArgsResult
		}
	}

	// No-op: no specs to resolve
	if len(args.Specs) == 0 {
		return nil
	}

	logWithId.Infof("Resolving containers (%d)", len(args.Specs))

	result.Specs = make([]worker.ContainerSpec, len(args.Specs))

	resolver := container.NewResolver(args.Arch)
	resolver.AuthFilePath = impl.AuthFilePath

	for _, s := range args.Specs {
		resolver.Add(s.ToVendorSourceSpec())
	}

	specs, err := resolver.Finish()

	if err != nil {
		result.JobError = clienterrors.New(clienterrors.ErrorContainerResolution, err.Error(), nil)
		return fmt.Errorf("Error resolving containers: %v", err)
	}

	for i, spec := range specs {
		result.Specs[i] = worker.ContainerSpecFromVendorSpec(spec)
	}

	return nil
}

// readContainerResolveArgsFromDynArgs reads the container resolve args from
// a BootcPreManifestJobResult stored in dynamic args at the given index.
func readContainerResolveArgsFromDynArgs(job worker.Job, dynArgsIdx int) (*worker.ContainerResolveJob, *clienterrors.Error) {
	if dynArgsIdx >= job.NDynamicArgs() {
		return nil, clienterrors.New(
			clienterrors.ErrorParsingDynamicArgs,
			"PreManifestDynArgsIdx is out of range",
			nil,
		)
	}

	var preManifestResult worker.BootcPreManifestJobResult
	if err := job.DynamicArgs(dynArgsIdx, &preManifestResult); err != nil {
		return nil, clienterrors.New(
			clienterrors.ErrorParsingDynamicArgs,
			"Error parsing BootcPreManifestJobResult from dynamic args: "+err.Error(),
			nil,
		)
	}

	if preManifestResult.JobError != nil {
		return nil, clienterrors.New(
			clienterrors.ErrorJobDependency,
			"BootcPreManifest dependency failed",
			preManifestResult.JobError.Reason,
		)
	}

	return preManifestResult.ContainerResolveJobArgs, nil
}
