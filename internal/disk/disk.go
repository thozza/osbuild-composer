// Disk package contains abstract data-types to define disk-related entities.
//
// PartitionTable, Partition and Filesystem types are currently defined.
// All of them can be 1:1 converted to osbuild.QEMUAssemblerOptions.
package disk

import (
	"sort"

	osbuild "github.com/osbuild/osbuild-composer/internal/osbuild1"
)

type PartitionTable struct {
	// Size of the disk.
	Size uint64 `hcl:"size"`
	UUID string `hcl:"uuid"`
	// Partition table type, e.g. dos, gpt.
	Type       string      `hcl:"type"`
	Partitions []Partition `hcl:"partition,block"`
}

type Partition struct {
	Start    uint64 `hcl:"start"`
	Size     uint64 `hcl:"size"`
	Type     string `hcl:"type"`
	Bootable bool   `hcl:"bootable"`
	// ID of the partition, dos doesn't use traditional UUIDs, therefore this
	// is just a string.
	UUID string `hcl:"uuid"`
	// If nil, the partition is raw; It doesn't contain a filesystem.
	Filesystem *Filesystem `hcl:"filesystem,optional"`
}

type Filesystem struct {
	Type string `hcl:"type"`
	// ID of the filesystem, vfat doesn't use traditional UUIDs, therefore this
	// is just a string.
	UUID       string `hcl:"uuid"`
	Label      string `hcl:"label"`
	Mountpoint string `hcl:"mount_point"`
	// The fourth field of fstab(5); fs_mntops
	FSTabOptions string `hcl:"fstab_options"`
	// The fifth field of fstab(5); fs_freq
	FSTabFreq uint64 `hcl:"fstab_freq"`
	// The sixth field of fstab(5); fs_passno
	FSTabPassNo uint64 `hcl:"fstab_passno"`
}

// Converts PartitionTable to osbuild.QEMUAssemblerOptions that encode
// the same partition table.
func (pt PartitionTable) QEMUAssemblerOptions() osbuild.QEMUAssemblerOptions {
	var partitions []osbuild.QEMUPartition
	for _, p := range pt.Partitions {
		partitions = append(partitions, p.QEMUPartition())
	}

	return osbuild.QEMUAssemblerOptions{
		Size:       pt.Size,
		PTUUID:     pt.UUID,
		PTType:     pt.Type,
		Partitions: partitions,
	}
}

// Generates org.osbuild.fstab stage options from this partition table.
func (pt PartitionTable) FSTabStageOptions() *osbuild.FSTabStageOptions {
	var options osbuild.FSTabStageOptions
	for _, p := range pt.Partitions {
		fs := p.Filesystem
		if fs == nil {
			continue
		}

		options.AddFilesystem(fs.UUID, fs.Type, fs.Mountpoint, fs.FSTabOptions, fs.FSTabFreq, fs.FSTabPassNo)
	}

	// sort the entries by PassNo to maintain backward compatibility
	sort.Slice(options.FileSystems, func(i, j int) bool {
		return options.FileSystems[i].PassNo < options.FileSystems[j].PassNo
	})

	return &options
}

// Returns the root partition (the partition whose filesystem has / as
// a mountpoint) of the partition table. Nil is returned if there's no such
// partition.
func (pt PartitionTable) RootPartition() *Partition {
	for _, p := range pt.Partitions {
		if p.Filesystem == nil {
			continue
		}

		if p.Filesystem.Mountpoint == "/" {
			return &p
		}
	}

	return nil
}

// Converts Partition to osbuild.QEMUPartition that encodes the same partition.
func (p Partition) QEMUPartition() osbuild.QEMUPartition {
	var fs *osbuild.QEMUFilesystem
	if p.Filesystem != nil {
		f := p.Filesystem.QEMUFilesystem()
		fs = &f
	}
	return osbuild.QEMUPartition{
		Start:      p.Start,
		Size:       p.Size,
		Type:       p.Type,
		Bootable:   p.Bootable,
		UUID:       p.UUID,
		Filesystem: fs,
	}
}

// Converts Filesystem to osbuild.QEMUFilesystem that encodes the same fs.
func (fs Filesystem) QEMUFilesystem() osbuild.QEMUFilesystem {
	return osbuild.QEMUFilesystem{
		Type:       fs.Type,
		UUID:       fs.UUID,
		Label:      fs.Label,
		Mountpoint: fs.Mountpoint,
	}
}
