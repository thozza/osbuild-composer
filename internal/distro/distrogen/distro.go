package distrogen

import (
	"sort"

	"github.com/osbuild/osbuild-composer/internal/blueprint"
	"github.com/osbuild/osbuild-composer/internal/distro"
	"github.com/osbuild/osbuild-composer/internal/rpmmd"
)

const (
	PackageSetPackages           string = "packages"
	PackageSetBuildPackages      string = "build_packages"
	PackageSetBootloaderPackages string = "bootloader_packages"
)

type Distro struct {
	NameValue             string `hcl:"name,label"`
	ModulePlatformIDValue string `hcl:"module_platform_id"`

	Arches []Arch `hcl:"arch,block"`

	Customizations *blueprint.Customizations   `hcl:"customizations,block"`
	PackageSetsMap map[string]rpmmd.PackageSet `hcl:"package_set"`
}

func (d *Distro) Name() string {
	return d.NameValue
}

func (d *Distro) ModulePlatformID() string {
	return d.ModulePlatformIDValue
}

func (d *Distro) ListArches() []string {
	archs := make([]string, 0, len(d.Arches))
	for _, arch := range d.Arches {
		archs = append(archs, arch.NameValue)
	}
	sort.Strings(archs)
	return archs
}

// GetArch returns an object representing the given architecture as support
// by this distro.
func (d *Distro) GetArch(arch string) (distro.Arch, error) {
	a := d.Arches[0] // TODO: FIX

	return &a, nil
}
