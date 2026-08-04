package version

import (
	"runtime/debug"
	"testing"
)

func TestAStampedBuildReportsWhatItWasGiven(t *testing.T) {
	original := value
	t.Cleanup(func() { value = original })
	value = "v1.4.0"

	if String() != "v1.4.0" {
		t.Fatalf("String() = %q", String())
	}
}

func TestAnUnstampedBuildStillNamesItself(t *testing.T) {
	original := value
	t.Cleanup(func() { value = original })
	value = ""

	if String() == "" {
		t.Fatal("an unstamped build could not say what it is")
	}
}

func TestFromBuildFallsBackToTheCommit(t *testing.T) {
	cases := []struct {
		name string
		info *debug.BuildInfo
		ok   bool
		want string
	}{
		{
			name: "no build info at all",
			ok:   false,
			want: "dev",
		},
		{
			name: "a go install build with a revision",
			info: &debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "8f4c1d2e9b7a5306f1c2d3e4"},
			}},
			ok:   true,
			want: "8f4c1d2e9b7a",
		},
		{
			name: "a revision shorter than the short form",
			info: &debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "8f4c1d2"},
			}},
			ok:   true,
			want: "8f4c1d2",
		},
		{
			name: "a revision that is empty",
			info: &debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: ""},
			}},
			ok:   true,
			want: "dev",
		},
		{
			name: "settings without a revision",
			info: &debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "GOOS", Value: "darwin"},
			}},
			ok:   true,
			want: "dev",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := fromBuild(tc.info, tc.ok)
			if got != tc.want {
				t.Fatalf("fromBuild = %q, want %q", got, tc.want)
			}
		})
	}
}
