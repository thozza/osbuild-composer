package main

import (
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/osbuild/images/pkg/bootc"
	"github.com/osbuild/osbuild-composer/internal/worker"
	"github.com/osbuild/osbuild-composer/internal/worker/clienterrors"
)

type BootcInfoResolveJobImpl struct{}

func (impl *BootcInfoResolveJobImpl) Run(job worker.Job) error {
	logWithId := logrus.WithField("jobId", job.Id())

	var args worker.BootcInfoResolveJob
	err := job.Args(&args)
	if err != nil {
		return err
	}

	result := worker.BootcInfoResolveJobResult{}
	defer func() {
		if r := recover(); r != nil {
			logWithId.Errorf("Recovered from panic in BootcInfoResolveJobImpl.Run: %v", r)
			result.JobError = clienterrors.New(clienterrors.ErrorJobPanicked, "Error resolving bootc info", r)
		}

		err := job.Finish(&result)
		if err != nil {
			logWithId.Errorf("Error reporting job result: %v", err)
		}
	}()

	logWithId.Infof("Resolving bootc container info (ref: %s, full_resolve: %v)", args.Ref, args.FullResolve)

	var info *bootc.Info
	if args.FullResolve {
		// Full resolution for the base container:
		// ResolveBootcInfo handles container lifecycle (start + stop)
		info, err = bootc.ResolveBootcInfo(args.Ref)
	} else {
		// Minimal resolution for the build container:
		// ResolveBootcBuildInfo handles container lifecycle (start + stop)
		info, err = bootc.ResolveBootcBuildInfo(args.Ref)
	}
	if err != nil {
		reason := fmt.Sprintf("failed to resolve bootc info for ref %q: %s", args.Ref, err.Error())
		result.JobError = clienterrors.New(clienterrors.ErrorBootcInfoResolve, reason, nil)
		return nil
	}

	// Convert vendor type to DTO for the job result
	result.Info, err = worker.BootcContainerInfoFromVendor(info)
	if err != nil {
		reason := fmt.Sprintf("failed to convert bootc info to DTO for ref %q: %s", args.Ref, err.Error())
		result.JobError = clienterrors.New(clienterrors.ErrorBootcInfoResolve, reason, nil)
		return nil
	}

	return nil
}
