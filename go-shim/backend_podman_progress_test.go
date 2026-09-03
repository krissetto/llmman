//go:build podman

package main

import "testing"

func TestArtifactProgressReducerUsesCumulativeOffset(t *testing.T) {
	const progressKey = "test:cumulative-offset"
	progressReset(progressKey, "pulling")
	defer func() {
		progressDone(progressKey)
		progressState.mu.Lock()
		delete(progressState.finished, progressKey)
		progressState.mu.Unlock()
	}()

	var reducer artifactProgressReducer
	progressAddTotal(progressKey, reducer.newArtifact("sha256:a", 100))

	positions := []int64{0}
	for _, offset := range []uint64{25, 25, 20, 100, 100} {
		progressAddCompleted(progressKey, reducer.update("sha256:a", offset))
		progressState.mu.Lock()
		positions = append(positions, progressState.entries[progressKey].completed)
		progressState.mu.Unlock()
	}

	want := []int64{0, 25, 25, 25, 100, 100}
	for i := range want {
		if positions[i] != want[i] {
			t.Fatalf("positions = %v, want %v", positions, want)
		}
	}
}

func TestArtifactProgressReducerReconcilesDoneAndRetryWithoutDoubleCount(t *testing.T) {
	var reducer artifactProgressReducer
	if got := reducer.newArtifact("sha256:a", 100); got != 100 {
		t.Fatalf("initial total delta = %d, want 100", got)
	}
	if got := reducer.update("sha256:a", 25); got != 25 {
		t.Fatalf("read delta = %d, want 25", got)
	}
	// A retry starts cumulative Offset over. Replayed bytes do not move the
	// aggregate, while movement beyond the prior frontier does.
	if got := reducer.newArtifact("sha256:a", 100); got != 0 {
		t.Fatalf("retry total delta = %d, want 0", got)
	}
	if got := reducer.update("sha256:a", 20); got != 0 {
		t.Fatalf("replayed retry delta = %d, want 0", got)
	}
	if got := reducer.update("sha256:a", 60); got != 35 {
		t.Fatalf("retry movement delta = %d, want 35", got)
	}
	// Done carries a cumulative offset but can carry OffsetUpdate == 0.
	if got := reducer.update("sha256:a", 100); got != 40 {
		t.Fatalf("done reconciliation delta = %d, want 40", got)
	}
	if got := reducer.update("sha256:a", 100); got != 0 {
		t.Fatalf("duplicate done delta = %d, want 0", got)
	}
}

func TestArtifactProgressReducerSkipCreditsOnlyOutstandingBytes(t *testing.T) {
	var reducer artifactProgressReducer
	if got := reducer.newArtifact("sha256:a", 100); got != 100 {
		t.Fatalf("total delta = %d, want 100", got)
	}
	if got := reducer.update("sha256:a", 25); got != 25 {
		t.Fatalf("read delta = %d, want 25", got)
	}
	total, completed := reducer.skip("sha256:a", 100)
	if total != 0 || completed != 75 {
		t.Fatalf("skip deltas = (%d, %d), want (0, 75)", total, completed)
	}

	// copy.Image may emit Skipped directly, without NewArtifact.
	total, completed = reducer.skip("sha256:b", 40)
	if total != 40 || completed != 40 {
		t.Fatalf("skip-only deltas = (%d, %d), want (40, 40)", total, completed)
	}
}
