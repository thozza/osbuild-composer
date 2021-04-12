package distrogen

import (
	"github.com/osbuild/osbuild-composer/internal/blueprint"
	"github.com/osbuild/osbuild-composer/internal/disk"
	"github.com/osbuild/osbuild-composer/internal/distro"
	"github.com/osbuild/osbuild-composer/internal/rpmmd"
)

type Arch struct {
	distro *Distro

	NameValue string `hcl:"name,label"`
	Legacy    string `hcl:"legacy"`
	UEFI      bool   `hcl:"uefi"`

	ImageTypes map[string]ImageType `hcl:"image_type"`

	Customizations *blueprint.Customizations   `hcl:"customizations,block"`
	PackageSetsMap map[string]rpmmd.PackageSet `hcl:"package_set"`
	PartitionTable *disk.PartitionTable        `hcl:"partition_table,block"`
}

func (a *Arch) Name() string {
	return a.NameValue
}

// Returns a sorted list of the names of the image types this architecture
// supports.
func (a *Arch) ListImageTypes() []string {
	return nil
}

// Returns an object representing a given image format for this architecture,
// on this distro.
func (a *Arch) GetImageType(imageType string) (distro.ImageType, error) {
	return nil, nil
}

// Returns the parent distro
func (a *Arch) Distro() distro.Distro {
	return nil
}
