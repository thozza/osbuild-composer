package rhel85

import (
	"github.com/osbuild/osbuild-composer/internal/distro"
	"github.com/osbuild/osbuild-composer/internal/rpmmd"
)

// getImageType_rhel_ec2 returns a rhel-ec2 (x86_64/aarch64) image type
func getImageType_rhel_ec2() distro.ImageType {
	rhelEc2 := &imageType{
		packageSets: map[string]rpmmd.PackageSet{
			"packages": {
				Include: []string{
					"@core",
					"cloud-init",
					"cloud-utils-growpart",
					"dhcp-client",
					"dracut-config-generic",
					"dracut-norescue",
					"gdisk",
					"grub2",
					"insights-client",
					"kernel",
					"NetworkManager",
					"NetworkManager-cloud-setup",
					"redhat-release",
					"redhat-release-eula",
					"rh-amazon-rhui-client",
					"rsync",
					"tar",
					"yum-utils",-
				},
				Exclude: []string{
					"aic94xx-firmware",
					"alsa-firmware",
					"alsa-lib",
					"alsa-tools-firmware",
					"biosdevname",
					"firewalld",
					"iprutils",
					"ivtv-firmware",
					"iwl1000-firmware",
					"iwl100-firmware",
					"iwl105-firmware",
					"iwl135-firmware",
					"iwl2000-firmware",
					"iwl2030-firmware",
					"iwl3160-firmware",
					"iwl3945-firmware",
					"iwl4965-firmware",
					"iwl5000-firmware",
					"iwl5150-firmware",
					"iwl6000-firmware",
					"iwl6000g2a-firmware",
					"iwl6000g2b-firmware",
					"iwl6050-firmware",
					"iwl7260-firmware",
					"libertas-sd8686-firmware",
					"libertas-sd8787-firmware",
					"libertas-usb8388-firmware",
					"plymouth",
				},
			},
		},
	}

	return rhelEc2
}

// getImageType_rhel_sap_ec2 returns a rhel-sap-ec2 image type
func getImageType_rhel_sap_ec2() distro.ImageType {
	rhelSapEc2 := &imageType{}

	return rhelSapEc2
}

// getImageType_rhel_ha_ec2 returns a rhel-ha-ec2 image type
func getImageType_rhel_ha_ec2() distro.ImageType {
	rhelHaEc2 := &imageType{}

	return rhelHaEc2
}
