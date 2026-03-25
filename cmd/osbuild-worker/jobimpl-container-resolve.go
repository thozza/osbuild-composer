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

	result := worker.ContainerResolveJobResult{
		Specs: make([]worker.ContainerSpec, len(args.Specs)),
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
