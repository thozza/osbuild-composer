package rpmmdtests

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/osbuild/osbuild-composer/internal/distro/test_distro"
	"github.com/osbuild/osbuild-composer/internal/rpmmd"
	"github.com/stretchr/testify/assert"
)

func getConfPaths(t *testing.T) []string {
	confPaths := []string{
		"./confpaths/priority1",
		"./confpaths/priority2",
	}
	var absConfPaths []string

	for _, path := range confPaths {
		absPath, err := filepath.Abs(path)
		assert.Nil(t, err)
		absConfPaths = append(absConfPaths, absPath)
	}

	return absConfPaths
}

func TestLoadRepositoriesExisting(t *testing.T) {
	confPaths := getConfPaths(t)
	type args struct {
		distro string
	}
	tests := []struct {
		name string
		args args
		want map[string][]string
	}{
		{
			name: "duplicate distro definition, load first encounter",
			args: args{
				distro: test_distro.TestDistroName,
			},
			want: map[string][]string{
				test_distro.TestArchName:  {"fedora-p1", "updates-p1", "fedora-modular-p1", "updates-modular-p1"},
				test_distro.TestArch2Name: {"fedora-p1", "updates-p1", "fedora-modular-p1", "updates-modular-p1"},
			},
		},
		{
			name: "single distro definition",
			args: args{
				distro: test_distro.TestDistro2Name,
			},
			want: map[string][]string{
				test_distro.TestArchName:  {"baseos-p2", "appstream-p2"},
				test_distro.TestArch2Name: {"baseos-p2", "appstream-p2", "google-compute-engine", "google-cloud-sdk"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rpmmd.LoadRepositories(confPaths, tt.args.distro)
			assert.Nil(t, err)

			for wantArch, wantRepos := range tt.want {
				gotArchRepos, exists := got[wantArch]
				assert.True(t, exists, "Expected '%s' arch in repos definition for '%s', but it does not exist", wantArch, tt.args.distro)

				var gotNames []string
				for _, r := range gotArchRepos {
					gotNames = append(gotNames, r.Name)
				}

				if !reflect.DeepEqual(gotNames, wantRepos) {
					t.Errorf("LoadRepositories() for %s/%s =\n got: %#v\n want: %#v", tt.args.distro, wantArch, gotNames, wantRepos)
				}
			}

		})
	}
}

func TestLoadRepositoriesNonExisting(t *testing.T) {
	confPaths := getConfPaths(t)
	repos, err := rpmmd.LoadRepositories(confPaths, "my-imaginary-distro")
	assert.Nil(t, repos)
	assert.NotNil(t, err)
}

func Test_LoadAllRepositories(t *testing.T) {
	confPaths := getConfPaths(t)

	distroReposMap, err := rpmmd.LoadAllRepositories(confPaths)
	assert.NotNil(t, distroReposMap)
	assert.Nil(t, err)
	assert.Equal(t, len(distroReposMap), 2)

	// test-distro
	testDistroRepos, exists := distroReposMap[test_distro.TestDistroName]
	assert.True(t, exists)
	assert.Equal(t, len(testDistroRepos), 2)

	// test-distro - arches
	for _, arch := range []string{test_distro.TestArchName, test_distro.TestArch2Name} {
		testDistroArchRepos, exists := testDistroRepos[arch]
		assert.True(t, exists)
		assert.Equal(t, len(testDistroArchRepos), 4)

		var repoNames []string
		for _, r := range testDistroArchRepos {
			repoNames = append(repoNames, r.Name)
		}

		wantRepos := []string{"fedora-p1", "updates-p1", "fedora-modular-p1", "updates-modular-p1"}

		if !reflect.DeepEqual(repoNames, wantRepos) {
			t.Errorf("LoadAllRepositories() for %s/%s =\n got: %#v\n want: %#v", test_distro.TestDistroName, arch, repoNames, wantRepos)
		}
	}

	// test-distro-2
	testDistro2Repos, exists := distroReposMap[test_distro.TestDistro2Name]
	assert.True(t, exists)
	assert.Equal(t, len(testDistro2Repos), 2)

	// test-distro-2 - arches
	wantRepos := map[string][]string{
		test_distro.TestArchName:  {"baseos-p2", "appstream-p2"},
		test_distro.TestArch2Name: {"baseos-p2", "appstream-p2", "google-compute-engine", "google-cloud-sdk"},
	}
	for _, arch := range []string{test_distro.TestArchName, test_distro.TestArch2Name} {
		testDistro2ArchRepos, exists := testDistro2Repos[arch]
		assert.True(t, exists)
		assert.Equal(t, len(testDistro2ArchRepos), len(wantRepos[arch]))

		var repoNames []string
		for _, r := range testDistro2ArchRepos {
			repoNames = append(repoNames, r.Name)
		}

		if !reflect.DeepEqual(repoNames, wantRepos[arch]) {
			t.Errorf("LoadAllRepositories() for %s/%s =\n got: %#v\n want: %#v", test_distro.TestDistro2Name, arch, repoNames, wantRepos[arch])
		}
	}
}

func TestPackageSetResolveConflictExclude(t *testing.T) {
	tests := []struct {
		got  rpmmd.PackageSet
		want rpmmd.PackageSet
	}{
		{
			got: rpmmd.PackageSet{
				Include: []string{"kernel", "microcode_ctl", "dnf"},
				Exclude: []string{"microcode_ctl"},
			},
			want: rpmmd.PackageSet{
				Include: []string{"kernel", "dnf"},
				Exclude: []string{"microcode_ctl"},
			},
		},
		{
			got: rpmmd.PackageSet{
				Include: []string{"kernel", "dnf"},
				Exclude: []string{"microcode_ctl"},
			},
			want: rpmmd.PackageSet{
				Include: []string{"kernel", "dnf"},
				Exclude: []string{"microcode_ctl"},
			},
		},
		{
			got: rpmmd.PackageSet{
				Include: []string{"kernel", "microcode_ctl", "dnf"},
				Exclude: []string{},
			},
			want: rpmmd.PackageSet{
				Include: []string{"kernel", "microcode_ctl", "dnf"},
				Exclude: []string{},
			},
		},
	}
	for idx, tt := range tests {
		t.Run(fmt.Sprintf("%d", idx), func(t *testing.T) {
			if !reflect.DeepEqual(tt.got.ResolveConflictsExclude(), tt.want) {
				t.Errorf("ResolveConflictExclude() returned unexpected result got: %#v\n want: %#v", tt.got.ResolveConflictsExclude(), tt.want)
			}
		})
	}
}

func TestChainPackageSets(t *testing.T) {
	tests := []struct {
		packageSetsChain []string
		packageSets      map[string]rpmmd.PackageSet
		repos            []rpmmd.RepoConfig
		packageSetsRepos map[string][]rpmmd.RepoConfig
		wantChainPkgSets []rpmmd.ChainPackageSet
		wantRepos        []rpmmd.RepoConfig
		err              bool
	}{
		// single transaction
		{
			packageSetsChain: []string{"os"},
			packageSets: map[string]rpmmd.PackageSet{
				"os": {
					Include: []string{"pkg1"},
					Exclude: []string{"pkg2"},
				},
			},
			repos: []rpmmd.RepoConfig{
				{
					Name:    "baseos",
					BaseURL: "https://example.org/baseos",
				},
				{
					Name:    "appstream",
					BaseURL: "https://example.org/appstream",
				},
			},
			wantChainPkgSets: []rpmmd.ChainPackageSet{
				{
					PackageSet: rpmmd.PackageSet{
						Include: []string{"pkg1"},
						Exclude: []string{"pkg2"},
					},
					Repos: []int{0, 1},
				},
			},
			wantRepos: []rpmmd.RepoConfig{
				{
					Name:    "baseos",
					BaseURL: "https://example.org/baseos",
				},
				{
					Name:    "appstream",
					BaseURL: "https://example.org/appstream",
				},
			},
		},
		// 2 transactions + package set specific repo
		{
			packageSetsChain: []string{"os", "blueprint"},
			packageSets: map[string]rpmmd.PackageSet{
				"os": {
					Include: []string{"pkg1"},
					Exclude: []string{"pkg2"},
				},
				"blueprint": {
					Include: []string{"pkg3"},
				},
			},
			repos: []rpmmd.RepoConfig{
				{
					Name:    "baseos",
					BaseURL: "https://example.org/baseos",
				},
				{
					Name:    "appstream",
					BaseURL: "https://example.org/appstream",
				},
			},
			packageSetsRepos: map[string][]rpmmd.RepoConfig{
				"blueprint": {
					{
						Name:    "user-repo",
						BaseURL: "https://example.org/user-repo",
					},
				},
			},
			wantChainPkgSets: []rpmmd.ChainPackageSet{
				{
					PackageSet: rpmmd.PackageSet{
						Include: []string{"pkg1"},
						Exclude: []string{"pkg2"},
					},
					Repos: []int{0, 1},
				},
				{
					PackageSet: rpmmd.PackageSet{
						Include: []string{"pkg3"},
					},
					Repos: []int{0, 1, 2},
				},
			},
			wantRepos: []rpmmd.RepoConfig{
				{
					Name:    "baseos",
					BaseURL: "https://example.org/baseos",
				},
				{
					Name:    "appstream",
					BaseURL: "https://example.org/appstream",
				},
				{
					Name:    "user-repo",
					BaseURL: "https://example.org/user-repo",
				},
			},
		},
		// 2 transactions + no package set specific repos
		{
			packageSetsChain: []string{"os", "blueprint"},
			packageSets: map[string]rpmmd.PackageSet{
				"os": {
					Include: []string{"pkg1"},
					Exclude: []string{"pkg2"},
				},
				"blueprint": {
					Include: []string{"pkg3"},
				},
			},
			repos: []rpmmd.RepoConfig{
				{
					Name:    "baseos",
					BaseURL: "https://example.org/baseos",
				},
				{
					Name:    "appstream",
					BaseURL: "https://example.org/appstream",
				},
			},
			wantChainPkgSets: []rpmmd.ChainPackageSet{
				{
					PackageSet: rpmmd.PackageSet{
						Include: []string{"pkg1"},
						Exclude: []string{"pkg2"},
					},
					Repos: []int{0, 1},
				},
				{
					PackageSet: rpmmd.PackageSet{
						Include: []string{"pkg3"},
					},
					Repos: []int{0, 1},
				},
			},
			wantRepos: []rpmmd.RepoConfig{
				{
					Name:    "baseos",
					BaseURL: "https://example.org/baseos",
				},
				{
					Name:    "appstream",
					BaseURL: "https://example.org/appstream",
				},
			},
		},
		// 3 transactions + package set specific repo used by 2nd and 3rd transaction
		{
			packageSetsChain: []string{"os", "blueprint", "blueprint2"},
			packageSets: map[string]rpmmd.PackageSet{
				"os": {
					Include: []string{"pkg1"},
					Exclude: []string{"pkg2"},
				},
				"blueprint": {
					Include: []string{"pkg3"},
				},
				"blueprint2": {
					Include: []string{"pkg4"},
				},
			},
			repos: []rpmmd.RepoConfig{
				{
					Name:    "baseos",
					BaseURL: "https://example.org/baseos",
				},
				{
					Name:    "appstream",
					BaseURL: "https://example.org/appstream",
				},
			},
			packageSetsRepos: map[string][]rpmmd.RepoConfig{
				"blueprint": {
					{
						Name:    "user-repo",
						BaseURL: "https://example.org/user-repo",
					},
				},
				"blueprint2": {
					{
						Name:    "user-repo",
						BaseURL: "https://example.org/user-repo",
					},
				},
			},
			wantChainPkgSets: []rpmmd.ChainPackageSet{
				{
					PackageSet: rpmmd.PackageSet{
						Include: []string{"pkg1"},
						Exclude: []string{"pkg2"},
					},
					Repos: []int{0, 1},
				},
				{
					PackageSet: rpmmd.PackageSet{
						Include: []string{"pkg3"},
					},
					Repos: []int{0, 1, 2},
				},
				{
					PackageSet: rpmmd.PackageSet{
						Include: []string{"pkg4"},
					},
					Repos: []int{0, 1, 2},
				},
			},
			wantRepos: []rpmmd.RepoConfig{
				{
					Name:    "baseos",
					BaseURL: "https://example.org/baseos",
				},
				{
					Name:    "appstream",
					BaseURL: "https://example.org/appstream",
				},
				{
					Name:    "user-repo",
					BaseURL: "https://example.org/user-repo",
				},
			},
		},
		// 3 transactions + package set specific repo used by 2nd and 3rd transaction
		// + 3rd transaction using another repo
		{
			packageSetsChain: []string{"os", "blueprint", "blueprint2"},
			packageSets: map[string]rpmmd.PackageSet{
				"os": {
					Include: []string{"pkg1"},
					Exclude: []string{"pkg2"},
				},
				"blueprint": {
					Include: []string{"pkg3"},
				},
				"blueprint2": {
					Include: []string{"pkg4"},
				},
			},
			repos: []rpmmd.RepoConfig{
				{
					Name:    "baseos",
					BaseURL: "https://example.org/baseos",
				},
				{
					Name:    "appstream",
					BaseURL: "https://example.org/appstream",
				},
			},
			packageSetsRepos: map[string][]rpmmd.RepoConfig{
				"blueprint": {
					{
						Name:    "user-repo",
						BaseURL: "https://example.org/user-repo",
					},
				},
				"blueprint2": {
					{
						Name:    "user-repo",
						BaseURL: "https://example.org/user-repo",
					},
					{
						Name:    "user-repo-2",
						BaseURL: "https://example.org/user-repo-2",
					},
				},
			},
			wantChainPkgSets: []rpmmd.ChainPackageSet{
				{
					PackageSet: rpmmd.PackageSet{
						Include: []string{"pkg1"},
						Exclude: []string{"pkg2"},
					},
					Repos: []int{0, 1},
				},
				{
					PackageSet: rpmmd.PackageSet{
						Include: []string{"pkg3"},
					},
					Repos: []int{0, 1, 2},
				},
				{
					PackageSet: rpmmd.PackageSet{
						Include: []string{"pkg4"},
					},
					Repos: []int{0, 1, 2, 3},
				},
			},
			wantRepos: []rpmmd.RepoConfig{
				{
					Name:    "baseos",
					BaseURL: "https://example.org/baseos",
				},
				{
					Name:    "appstream",
					BaseURL: "https://example.org/appstream",
				},
				{
					Name:    "user-repo",
					BaseURL: "https://example.org/user-repo",
				},
				{
					Name:    "user-repo-2",
					BaseURL: "https://example.org/user-repo-2",
				},
			},
		},
		// Error: 3 transactions + 3rd one not using repo used by 2nd one
		{
			packageSetsChain: []string{"os", "blueprint", "blueprint2"},
			packageSets: map[string]rpmmd.PackageSet{
				"os": {
					Include: []string{"pkg1"},
					Exclude: []string{"pkg2"},
				},
				"blueprint": {
					Include: []string{"pkg3"},
				},
				"blueprint2": {
					Include: []string{"pkg4"},
				},
			},
			repos: []rpmmd.RepoConfig{
				{
					Name:    "baseos",
					BaseURL: "https://example.org/baseos",
				},
				{
					Name:    "appstream",
					BaseURL: "https://example.org/appstream",
				},
			},
			packageSetsRepos: map[string][]rpmmd.RepoConfig{
				"blueprint": {
					{
						Name:    "user-repo",
						BaseURL: "https://example.org/user-repo",
					},
				},
				"blueprint2": {
					{
						Name:    "user-repo2",
						BaseURL: "https://example.org/user-repo2",
					},
				},
			},
			err: true,
		},
		// Error: requested package set name to chain not defined in provided pkg sets
		{
			packageSetsChain: []string{"os", "blueprint"},
			packageSets: map[string]rpmmd.PackageSet{
				"os": {
					Include: []string{"pkg1"},
					Exclude: []string{"pkg2"},
				},
			},
			err: true,
		},
	}
	for idx, tt := range tests {
		t.Run(fmt.Sprintf("%d", idx), func(t *testing.T) {
			gotChainPackageSets, gotRepos, err := rpmmd.ChainPackageSets(tt.packageSetsChain, tt.packageSets, tt.repos, tt.packageSetsRepos)
			if tt.err {
				assert.NotNilf(t, err, "expected an error, but got 'nil' instead")
				assert.Nilf(t, gotChainPackageSets, "got non-nill []rpmmd.ChainPackageSet, but expected an error")
				assert.Nilf(t, gotRepos, "got non-nill []rpmmd.RepoConfig, but expected an error")
			} else {
				assert.Nilf(t, err, "expected 'nil', but got error instead")
				assert.NotNilf(t, gotChainPackageSets, "expected non-nill []rpmmd.ChainPackageSet, but got 'nil' instead")
				assert.NotNilf(t, gotRepos, "expected non-nill []rpmmd.RepoConfig, but got 'nil' instead")

				assert.Equal(t, tt.wantChainPkgSets, gotChainPackageSets)
				assert.Equal(t, tt.wantRepos, gotRepos)
			}
		})
	}
}
