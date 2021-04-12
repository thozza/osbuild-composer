package distrogen

type Definitions struct {
	Distros    []Distro    `hcl:"distro,block"`
	Arches     []Arch      `hcl:"arch"`
	ImageTypes []ImageType `hcl:"image_type"`
}

func LoadDefinitions() (*Definitions, error) {
	return nil, nil

}
