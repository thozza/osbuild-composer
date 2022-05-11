package target

type KojiTargetOptions struct {
	Filename        string `json:"filename"`
	UploadDirectory string `json:"upload_directory"`
	Server          string `json:"server"`
}

func (KojiTargetOptions) isTargetOptions() {}

func NewKojiTarget(options *KojiTargetOptions) *Target {
	return newTarget("org.osbuild.koji", options)
}

type KojiTargetResultOptions struct {
	HostOS    string `json:"host_os"`
	Arch      string `json:"arch"`
	ImageHash string `json:"image_hash"`
	ImageSize uint64 `json:"image_size"`
}

func (KojiTargetResultOptions) isTargetResultOptions() {}

func NewKojiTargetResult(options *KojiTargetResultOptions) *TargetResult {
	return newTargetResult("org.osbuild.koji", options)
}
