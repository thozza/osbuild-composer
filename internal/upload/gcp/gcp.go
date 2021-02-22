package gcp

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	cloudbuild "cloud.google.com/go/cloudbuild/apiv1"
	"cloud.google.com/go/storage"
	"github.com/golang/protobuf/ptypes"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/iam/v1"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	cloudbuildpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
	"google.golang.org/protobuf/types/known/durationpb"
)

type GCP struct {
	creds *google.Credentials
}

func New(credentials []byte) (*GCP, error) {
	scopes := []string{
		compute.ComputeScope,   // permissions to image
		storage.ScopeReadWrite, // file upload
		iam.CloudPlatformScope, // creation of custom role
	}
	scopes = append(scopes, cloudbuild.DefaultAuthScopes()...) // image import

	var getCredsFunc func() (*google.Credentials, error)
	if credentials != nil {
		getCredsFunc = func() (*google.Credentials, error) {
			return google.CredentialsFromJSON(
				context.Background(),
				credentials,
				scopes...,
			)
		}
	} else {
		getCredsFunc = func() (*google.Credentials, error) {
			return google.FindDefaultCredentials(
				context.Background(),
				scopes...,
			)
		}
	}

	creds, err := getCredsFunc()
	if err != nil {
		return nil, fmt.Errorf("failed to get Google credentials: %v", err)
	}

	return &GCP{creds}, nil
}

// GetProjectID returns a string with the Project ID of the project, used for
// all GCP operations.
func (g *GCP) GetProjectID() string {
	return g.creds.ProjectID
}

// Upload uploads an OS image to  specified Cloud Storage bucket and object.
// The bucket must exist. MD5 sum of the image file and uploaded object is
// compared after the upload to verify the integrity of the uploaded image.
//
// Uses:
//	- Storage API
func (g *GCP) Upload(filename, bucket, object string) error {
	ctx := context.Background()

	storageClient, err := storage.NewClient(ctx, option.WithCredentials(g.creds))
	if err != nil {
		return fmt.Errorf("failed to get Storage client: %v", err)
	}
	defer storageClient.Close()

	// Open the image file
	imageFile, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("cannot open the image: %v", err)
	}
	defer imageFile.Close()

	// Compute MD5 checksum of the image file for later verification
	imageFileHash := md5.New()
	if _, err := io.Copy(imageFileHash, imageFile); err != nil {
		return fmt.Errorf("cannot create md5 of the image: %v", err)
	}
	// Move the cursor of opened file back to the start
	if _, err := imageFile.Seek(0, 0); err != nil {
		return fmt.Errorf("cannot seek the image: %v", err)
	}

	// Upload the image
	// The Bucket MUST exist and be of a STANDARD storage class
	obj := storageClient.Bucket(bucket).Object(object)
	wc := obj.NewWriter(ctx)
	fmt.Printf("[GCP] 🚀 Uploading image to: %s/%s\n", bucket, object)
	if _, err = io.Copy(wc, imageFile); err != nil {
		return fmt.Errorf("uploading the image failed: %v", err)
	}

	if err := wc.Close(); err != nil {
		return fmt.Errorf("Writer.Close: %v", err)
	}

	// Verify the MD5 sum of the uploaded file
	objAttrs, err := obj.Attrs(ctx)
	if err != nil {
		return fmt.Errorf("cannot get uploaded object attributed: %v", err)
	}
	objChecksum := objAttrs.MD5
	fileChecksum := imageFileHash.Sum(nil)
	if !bytes.Equal(objChecksum, fileChecksum) {
		return fmt.Errorf("error during image upload. the image seems to be corrupted")
	}

	return nil
}

// Import imports a previously uploaded image by submitting a Cloud Build API
// job. The job builds an image into Compute Node from an image uploaded to the
// storage.
//
// The source image file is deleted from the storage bucket after a successfull
// image import. Also all cache files created as part of the image import are
// deleted from the respective storage bucket.
//
// bucket - Google storage bucket name with the uploaded image
// object - Google storage object name of the uploaded image
// imageName - Desired image name after the import. This must be unique within the whole project.
// os - Specifies the OS type used when installing GCP guest tools.
//      If empty (""), then the image is imported without the installation of GCP guest tools.
// 		Valid values are: centos-7, centos-8, debian-8, debian-9, opensuse-15, rhel-6,
//                        rhel-6-byol, rhel-7, rhel-7-byol, rhel-8, rhel-8-byol, sles-12,
//                        sles-12-byol, sles-15, sles-15-byol, sles-sap-12, sles-sap-12-byol,
//                        sles-sap-15, sles-sap-15-byol, ubuntu-1404, ubuntu-1604, ubuntu-1804,
//                        ubuntu-2004, windows-10-x64-byol, windows-10-x86-byol,
//                        windows-2008r2, windows-2008r2-byol, windows-2012, windows-2012-byol,
//                        windows-2012r2, windows-2012r2-byol, windows-2016, windows-2016-byol,
//                        windows-2019, windows-2019-byol, windows-7-x64-byol,
//                        windows-7-x86-byol, windows-8-x64-byol, windows-8-x86-byol
// region - A valid region where the resulting image should be located. If empty,
//          the multi-region location closest to the source is chosen automatically.
//          See: https://cloud.google.com/storage/docs/locations
//
// Uses:
//	- Cloud Build API
func (g *GCP) Import(bucket, object, imageName, os, region string) error {
	ctx := context.Background()
	cloudbuildClient, err := cloudbuild.NewClient(ctx, option.WithCredentials(g.creds))
	if err != nil {
		return fmt.Errorf("failed to get Cloud Build client: %v", err)
	}
	defer cloudbuildClient.Close()

	buildStepArgs := []string{
		fmt.Sprintf("-source_file=gs://%s/%s", bucket, object),
		fmt.Sprintf("-image_name=%s", imageName),
		"-timeout=7000s",
		"-client_id=api",
	}
	if region != "" {
		buildStepArgs = append(buildStepArgs, fmt.Sprintf("-storage_location=%s", region))
	}
	if os != "" {
		buildStepArgs = append(buildStepArgs, fmt.Sprintf("-os=%s", os))
	} else {
		// This effectively marks the image as non-bootable for the import process,
		// but it has no effect on the later use or booting in Compute Engine other
		// than the GCP guest tools not being installed.
		buildStepArgs = append(buildStepArgs, "-data_disk")
	}

	imageBuild := &cloudbuildpb.Build{
		Steps: []*cloudbuildpb.BuildStep{{
			Name: "gcr.io/compute-image-tools/gce_vm_image_import:release",
			Args: buildStepArgs,
		}},
		Tags: []string{
			"gce-daisy",
			"gce-daisy-image-import",
		},
		Timeout: durationpb.New(time.Second * 7200),
	}

	createBuildReq := &cloudbuildpb.CreateBuildRequest{
		ProjectId: g.creds.ProjectID,
		Build:     imageBuild,
	}
	fmt.Printf("[GCP] 📥 Importing image into Compute Node as '%s'\n", imageName)
	resp, err := cloudbuildClient.CreateBuild(ctx, createBuildReq)
	if err != nil {
		return fmt.Errorf("failed to create image import build job: %v", err)
	}

	// Get the returned Build struct
	buildOpMetadata := &cloudbuildpb.BuildOperationMetadata{}
	if err := ptypes.UnmarshalAny(resp.Metadata, buildOpMetadata); err != nil {
		return err
	}
	imageBuild = buildOpMetadata.Build

	fmt.Printf("[GCP] 📜 Image import log URL: %s\n", imageBuild.LogUrl)
	fmt.Printf("[GCP] 🤔 Image import build status: %+v\n", imageBuild.Status)

	getBuldReq := &cloudbuildpb.GetBuildRequest{
		ProjectId: imageBuild.ProjectId,
		Id:        imageBuild.Id,
	}

	// Wait for the build to finish
	fmt.Println("[GCP] 🥱 Waiting for the image import to finish")
	for {
		imageBuild, err = cloudbuildClient.GetBuild(ctx, getBuldReq)
		if err != nil {
			return fmt.Errorf("failed to get the build info: %v", err)
		}
		// The build finished
		if imageBuild.Status != cloudbuildpb.Build_WORKING && imageBuild.Status != cloudbuildpb.Build_QUEUED {
			break
		}
		time.Sleep(time.Second * 30)
	}

	fmt.Printf("[GCP] 🎉 Image import finished with status: %s\n", imageBuild.Status)

	// Clean up cache files created by the Image Import Build job
	if err = g.ImageImportStorageCleanup(bucket, object, imageName); err != nil {
		fmt.Printf("storage cleanup failed: %v", err)
	}

	if imageBuild.Status != cloudbuildpb.Build_SUCCESS {
		return fmt.Errorf("image import didn't finish successfully: %s", imageBuild.Status)
	}

	fmt.Printf("[GCP] 💿 Image URL: https://console.cloud.google.com/compute/imagesDetail/projects/%s/global/images/%s\n", g.creds.ProjectID, imageName)

	return nil
}

// ImageImportStorageCleanup deletes all objects created as part of an Image
// import into Compute Engine and the related Build Job. It also deletes the
// source image file, which has been used for image import.
//
// The Build job stores a copy of the to-be-imported image in a region specific
// bucket, along with the Build job logs.
//
// Uses:
//	- Compute Engine API
//	- Storage API
func (g *GCP) ImageImportStorageCleanup(bucket, object, imageName string) error {
	ctx := context.Background()

	storageClient, err := storage.NewClient(ctx, option.WithCredentials(g.creds))
	if err != nil {
		return fmt.Errorf("failed to get Storage client: %v", err)
	}
	defer storageClient.Close()

	computeService, err := compute.NewService(ctx, option.WithCredentials(g.creds))
	if err != nil {
		return fmt.Errorf("failed to get Compute Engine client: %v", err)
	}

	// Clean up the cache bucket
	imageGetCall := computeService.Images.Get(g.creds.ProjectID, imageName)
	image, err := imageGetCall.Do()
	if err != nil {
		// Without the image, we can not determine which objects to delete, just return
		return fmt.Errorf("failed to get image: %v", err)
	}

	// Determine the regular expression to match files related to the specific Image Import
	// e.g. "https://www.googleapis.com/compute/v1/projects/ascendant-braid-303513/zones/europe-west1-b/disks/disk-d7tr4"
	// e.g. "https://www.googleapis.com/compute/v1/projects/ascendant-braid-303513/zones/europe-west1-b/disks/disk-l7s2w-1"
	// Needed is only the part between "disk-" and possible "-<num>"/"EOF"
	ss := strings.Split(image.SourceDisk, "/")
	srcDiskName := ss[len(ss)-1]
	ss = strings.Split(srcDiskName, "-")
	if len(ss) < 2 {
		return fmt.Errorf("unexpected source disk name '%s', can not clean up storage", srcDiskName)
	}
	scrDiskSuffix := ss[1]
	// e.g. "gce-image-import-2021-02-05T17:27:40Z-2xhp5/daisy-import-image-20210205-17:27:43-s6l0l/logs/daisy.log"
	reStr := fmt.Sprintf("gce-image-import-.+-%s", scrDiskSuffix)
	cacheFilesRe := regexp.MustCompile(reStr)

	buckets := storageClient.Buckets(ctx, g.creds.ProjectID)
	for {
		bkt, err := buckets.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("failure while iterating over storage buckets: %v", err)
		}

		// Check all buckets created by the Image Import Build jobs
		// These are named e.g. "<project_id>-daisy-bkt-eu" - "ascendant-braid-303513-daisy-bkt-eu"
		if strings.HasPrefix(bkt.Name, fmt.Sprintf("%s-daisy-bkt", g.creds.ProjectID)) {
			objects := storageClient.Bucket(bkt.Name).Objects(ctx, nil)
			for {
				obj, err := objects.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					// Do not return, just log, to clean up as much as possible!
					fmt.Printf("ERROR: failure while iterating over storage objects: %v", err)
					break
				}
				if cacheFilesRe.FindString(obj.Name) != "" {
					o := storageClient.Bucket(bkt.Name).Object(obj.Name)
					fmt.Printf("[GCP] 🧹 Deleting image import job file '%s'\n", obj.Name)
					if err = o.Delete(ctx); err != nil {
						// Do not return, just log, to clean up as much as possible!
						fmt.Printf("ERROR: failed to delete storage object: %v", err)
					}
				}
			}
		}
	}

	imageFileObject := storageClient.Bucket(bucket).Object(object)
	fmt.Printf("[GCP] 🧹 Deleting image file from Storage: %s/%s\n", bucket, object)
	if err = imageFileObject.Delete(ctx); err != nil {
		return fmt.Errorf("failed to delete image file object: %v", err)
	}

	return nil
}

// Share shares the specified Compute Engine image with list of accounts.
//
// "shareWith" is a list of accounts to share the image with. Items can be one
// of the following options:
//
// - `user:{emailid}`: An email address that represents a specific
//	 Google account. For example, `alice@example.com`.
//
// - `serviceAccount:{emailid}`: An email address that represents a
//   service account. For example, `my-other-app@appspot.gserviceaccount.com`.
//
// - `group:{emailid}`: An email address that represents a Google group.
//   For example, `admins@example.com`.
//
// - `domain:{domain}`: The G Suite domain (primary) that represents all
//   the users of that domain. For example, `google.com` or `example.com`.
//
// Uses:
//	- Compute Engine API
func (g *GCP) Share(imageName string, shareWith []string) error {
	ctx := context.Background()

	computeService, err := compute.NewService(ctx, option.WithCredentials(g.creds))
	if err != nil {
		return fmt.Errorf("failed to get Compute Engine client: %v", err)
	}

	// Standard role to enable account to view and use a specific Image
	imageDesiredRole := "roles/compute.imageUser"

	// Get the current Policy set on the Image
	existingPolicyCall := computeService.Images.GetIamPolicy(g.creds.ProjectID, imageName)
	policy, err := existingPolicyCall.Do()
	if err != nil {
		return fmt.Errorf("failed to get image's policy: %v", err)
	}

	// Add new members, who can use the image
	// Completely override the old policy
	userBinding := &compute.Binding{
		Members: shareWith,
		Role:    imageDesiredRole,
	}
	newPolicy := &compute.Policy{
		Bindings: []*compute.Binding{userBinding},
		Etag:     policy.Etag,
	}
	req := &compute.GlobalSetPolicyRequest{
		Policy: newPolicy,
	}
	newPolicyCall := computeService.Images.SetIamPolicy(g.creds.ProjectID, imageName, req)
	fmt.Printf("[GCP] Sharing the image with: %+v\n", shareWith)
	policy, err = newPolicyCall.Do()
	if err != nil {
		return fmt.Errorf("failed to set new image policy: %v", err)
	}

	// Users won't see the shared image in their images.list requests, unless
	// they are also granted a specific "imagesList" role on the project. If you
	// don't need users to be able to view the list of shared images, this
	// step can be skipped.
	//
	// Downside of granting the "imagesList" role to a project is that the user
	// will be able to list all available images in the project, even those that
	// they can't use because of insufficient permissions.
	//
	// Even without the ability to view / list shared images, the user can still
	// create a Compute Node instance using the image via API or 'gcloud' tool.
	//
	// Custom role to enable account to only list images in the project.
	// Without this role, the account won't be able to list and see the image
	// in the GCP Web UI.

	// For now, the decision is that the account should not get any role to the
	// project, where the image has been imported.
	// if err = g.GrantImagesListOnProjectWithContect(ctx, shareWith); err != nil {
	// 	return fmt.Errorf("failed to grant ImageList role on the project: %v", err)
	// }

	return nil
}

// GrantImagesListOnProjectWithContect grants an "imagesList" role on the
// respective project to the list of accounts, so that they can list and see
// shared imagess.
//
// "shareWith" is a list of accounts to share the image with. Items can be one
// of the following options:
//
// - `user:{emailid}`: An email address that represents a specific
//	 Google account. For example, `alice@example.com`.
//
// - `serviceAccount:{emailid}`: An email address that represents a
//   service account. For example, `my-other-app@appspot.gserviceaccount.com`.
//
// - `group:{emailid}`: An email address that represents a Google group.
//   For example, `admins@example.com`.
//
// - `domain:{domain}`: The G Suite domain (primary) that represents all
//   the users of that domain. For example, `google.com` or `example.com`.
//
// Uses:
//	- Cloud Resource Manager API
//  - AIM API
func (g *GCP) GrantImagesListOnProjectWithContect(ctx context.Context, shareWith []string) error {
	iamService, err := iam.NewService(ctx, option.WithCredentials(g.creds))
	if err != nil {
		return fmt.Errorf("failed to get IAM client: %v", err)
	}

	resourceMngrService, err := cloudresourcemanager.NewService(ctx, option.WithCredentials(g.creds))
	if err != nil {
		return fmt.Errorf("failed to get Cloud Resource Manager client: %v", err)
	}

	projectDesiredRoleID := "compute.imagesList"
	projectDesiredRole := fmt.Sprintf(
		"projects/%s/roles/%s",
		g.creds.ProjectID,
		projectDesiredRoleID,
	)

	// Check if the Images List role exists and if not, create it
	getRolesCall := iamService.Roles.Get(projectDesiredRole)
	imagesListRole, err := getRolesCall.Do()
	// TODO: it may make sense to verify, that the role is active (not deleted/disabled)
	if err != nil {
		// Role was not found, create it
		if strings.Contains(err.Error(), "Error 404") {
			imagesListRole = &iam.Role{
				IncludedPermissions: []string{
					"compute.images.list",
					"resourcemanager.projects.get",
				},
				Description: "Custom role to allow listing of Images",
				Title:       "Compute Images List",
				Stage:       "GA",
			}
			createRoleReq := &iam.CreateRoleRequest{
				Role:   imagesListRole,
				RoleId: projectDesiredRoleID,
			}
			createRoleCall := iamService.Projects.Roles.Create(
				fmt.Sprintf("projects/%s", g.creds.ProjectID),
				createRoleReq,
			)
			fmt.Printf("[GCP] 🛡️  Creating a new custom role '%s'\n", projectDesiredRole)
			imagesListRole, err = createRoleCall.Do()
			if err != nil {
				return fmt.Errorf("failed to create a new custom role: %v", err)
			}
		} else {
			return fmt.Errorf("failed to get role '%s': %v", projectDesiredRole, err)
		}
	}

	// Get the project's policy
	getProjectPolicyCall := resourceMngrService.Projects.GetIamPolicy(
		g.creds.ProjectID,
		&cloudresourcemanager.GetIamPolicyRequest{},
	)
	projectPolicy, err := getProjectPolicyCall.Do()
	if err != nil {
		return fmt.Errorf("failed to get project's policy: %v", err)
	}

	// Go through the existing project policy and look up a binding for specific
	// role "projects/<project_id>/roles/compute.imagesList" and add the acount
	// to the list, if it is not already there.
	bindings := projectPolicy.Bindings
	roleBindingExists := false
	for _, b := range bindings {
		if b.Role == projectDesiredRole {
			roleBindingExists = true
			// check each account, tha the image should be shared with
			for _, shareAccount := range shareWith {
				// check if the account is already member of the role
				found := false
				for _, m := range b.Members {
					if m == shareAccount {
						found = true
						break
					}
				}
				// the account is not in the list, add it
				if !found {
					fmt.Printf("[GCP] 🛡️  Granting new role '%s' to '%s' in project '%s'\n", projectDesiredRole, shareAccount, g.creds.ProjectID)
					b.Members = append(b.Members, shareAccount)
				}
			}
		}
	}
	// If a binding for the desired role does not exist in the project's policy
	// yet, then no new members were added by the previous code block.
	// Create a new binding and add it to the list of bindings of the policy.
	if !roleBindingExists {
		fmt.Printf("[GCP] 🛡️  Granting new role '%s' to '%v' in project '%s'\n", projectDesiredRole, shareWith, g.creds.ProjectID)
		bindings = append(bindings, &cloudresourcemanager.Binding{
			Members: shareWith,
			Role:    projectDesiredRole,
		})
	}

	// Update the project's policy with new list of bindings
	setProjectPolicyReq := &cloudresourcemanager.SetIamPolicyRequest{
		Policy: &cloudresourcemanager.Policy{
			Bindings: bindings,
			Etag:     projectPolicy.Etag,
		},
	}
	setProjectPolicyCall := resourceMngrService.Projects.SetIamPolicy(g.creds.ProjectID, setProjectPolicyReq)
	projectPolicy, err = setProjectPolicyCall.Do()
	if err != nil {
		return fmt.Errorf("failed to set new project policy: %v", err)
	}

	return nil
}
