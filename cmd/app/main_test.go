package main

import "testing"

func TestReleaseRevisionIncludesBuildIdentity(t *testing.T) {
	previousVersion, previousCommit, previousBuildDate := Version, Commit, BuildDate
	t.Cleanup(func() {
		Version, Commit, BuildDate = previousVersion, previousCommit, previousBuildDate
	})

	Version = " v1.1.1 "
	Commit = " abc123 "
	BuildDate = " 2026-07-27T12:00:00Z "

	if got, want := releaseRevision(), "v1.1.1|abc123|2026-07-27T12:00:00Z"; got != want {
		t.Fatalf("releaseRevision() = %q, want %q", got, want)
	}
}
