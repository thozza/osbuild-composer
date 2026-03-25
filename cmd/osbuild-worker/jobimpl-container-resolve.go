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

// resolveContainerSpecs resolves container specs using the container resolver.
// Extracted as a variable to allow test injection.
var resolveContainerSpecs = func(arch, authFilePath string, specs []worker.ContainerSpec) ([]container.Spec, error) {
	resolver := container.NewResolver(arch)
	resolver.AuthFilePath = authFilePath
	for _, s := range specs {
		resolver.Add(s.ToVendorSourceSpec())
	}
	return resolver.Finish()
}

func (impl *ContainerResolveJobImpl) Run(job worker.Job) error {
	logWithId := logrus.WithField("jobId", job.Id())
	var args worker.ContainerResolveJob
	err := job.Args(&args)
	if err != nil {
		return err
	}

	result := worker.ContainerResolveJobResult{}

	// If static args are empty, read from dynamic args (BootcPreManifest result)
	if len(args.Specs) == 0 && args.PreManifestDynArgsIdx != nil {
		idx := *args.PreManifestDynArgsIdx
		if idx > job.NDynamicArgs()-1 {
			result.JobError = clienterrors.New(
				clienterrors.ErrorParsingDynamicArgs,
				"PreManifestDynArgsIdx is out of range", nil,
			)
			err = job.Finish(&result)
			if err != nil {
				return fmt.Errorf("Error reporting job result: %v", err)
			}
			return nil
		}
		var preManifestResult worker.BootcPreManifestJobResult
		if err := job.DynamicArgs(idx, &preManifestResult); err != nil {
			result.JobError = clienterrors.New(
				clienterrors.ErrorParsingDynamicArgs,
				"Error parsing BootcPreManifestJobResult from dynamic args: "+err.Error(), nil,
			)
			err = job.Finish(&result)
			if err != nil {
				return fmt.Errorf("Error reporting job result: %v", err)
			}
			return nil
		}
		if preManifestResult.JobError != nil {
			result.JobError = clienterrors.New(
				clienterrors.ErrorJobDependency,
				"BootcPreManifest dependency failed", preManifestResult.JobError.Reason,
			)
			err = job.Finish(&result)
			if err != nil {
				return fmt.Errorf("Error reporting job result: %v", err)
			}
			return nil
		}
		if preManifestResult.ContainerResolveJobArgs != nil {
			args = *preManifestResult.ContainerResolveJobArgs
		}
	}

	result.Specs = make([]worker.ContainerSpec, len(args.Specs))

	// No specs to resolve
	if len(args.Specs) == 0 {
		logWithId.Infof("No containers to resolve")
		err = job.Finish(&result)
		if err != nil {
			return fmt.Errorf("Error reporting job result: %v", err)
		}
		return nil
	}

	logWithId.Infof("Resolving containers (%d)", len(args.Specs))

	specs, err := resolveContainerSpecs(args.Arch, impl.AuthFilePath, args.Specs)

	if err != nil {
		result.JobError = clienterrors.New(clienterrors.ErrorContainerResolution, err.Error(), nil)
	} else {
		for i, spec := range specs {
			result.Specs[i] = worker.ContainerSpecFromVendorSpec(spec)
		}
	}

	err = job.Finish(&result)
	if err != nil {
		return fmt.Errorf("Error reporting job result: %v", err)
	}

	return nil
}
