package distrogen

import (
	"github.com/osbuild/osbuild-composer/internal/blueprint"
	"github.com/osbuild/osbuild-composer/internal/disk"
	"github.com/osbuild/osbuild-composer/internal/distro"
	"github.com/osbuild/osbuild-composer/internal/rpmmd"
)

type ImageType struct {
	arch *Arch

	NameValue      string `hcl:"name,label"`
	FilenameValue  string `hcl:"filename"`
	MIMETypeValue  string `hcl:"mime_type"`
	OSTreeRefValue string `hcl:"ostree_ref_value"`
	SizeValue      uint64 `hcl:"size"`
	DefaultTarget  string `hcl:"default_target"`
	// kernel options?
	Bootable  bool `hcl:"bootable"`
	RpmOstree bool `hcl:"rpm_ostree"`

	Customizations *blueprint.Customizations   `hcl:"customizations,block"`
	PackageSetsMap map[string]rpmmd.PackageSet `hcl:"package_set"`
	PartitionTable *disk.PartitionTable        `hcl:"partition_table,block"`

	OsbuildVersion *int `hcl:"osbuild_version,optional"` // default to version 2

	// TODO: figure out how to specify Exports (export stages)
}

func (i *ImageType) Name() string {
	return i.NameValue
}

// Returns the parent architecture
func (i *ImageType) Arch() distro.Arch {
	if i.arch != nil {
		return i.arch
	}

	return &Arch{}
}

// Returns the canonical filename for the image type.
func (i *ImageType) Filename() string {
	return i.FilenameValue
}

// Retrns the MIME-type for the image type.
func (i *ImageType) MIMEType() string {
	return i.MIMETypeValue
}

// Returns the default OSTree ref for the image type.
func (i *ImageType) OSTreeRef() string {
	return i.OSTreeRefValue
}

// Returns the proper image size for a given output format. If the input size
// is 0 the default value for the format will be returned.
func (i *ImageType) Size(size uint64) uint64 {
	return i.SizeValue
}

// Returns the sets of packages to include and exclude when building the image.
// Indexed by a string label. How each set is labeled and used depends on the
// image type.
func (i *ImageType) PackageSets(bp blueprint.Blueprint) map[string]rpmmd.PackageSet {
	return nil
}

// Returns the names of the stages that will produce the build output.
func (i *ImageType) Exports() []string {
	return nil
}

// Returns an osbuild manifest, containing the sources and pipeline necessary
// to build an image, given output format with all packages and customizations
// specified in the given blueprint. The packageSpecSets must be labelled in
// the same way as the originating PackageSets.
func (i *ImageType) Manifest(b *blueprint.Customizations, options distro.ImageOptions, repos []rpmmd.RepoConfig, packageSpecSets map[string][]rpmmd.PackageSpec, seed int64) (distro.Manifest, error) {

	if *i.OsbuildVersion == 2 {
		return nil, nil
	} else if *i.OsbuildVersion == 1 {
		return nil, nil
	}
	return nil, nil
}
