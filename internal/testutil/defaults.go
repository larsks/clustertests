package testutil

import "time"

// Defaults for the settings that can be overridden through the environment;
// keep them in sync with the table in README.md.
const (
	// DefaultUnschedulablePodTimeout is how long a pod may be unschedulable
	// before it is reported (UNSCHEDULABLE_POD_TIMEOUT).
	DefaultUnschedulablePodTimeout = 10 * time.Minute

	// DefaultRecentTerminationWindow is how recently a container must have
	// been OOMKilled or restarted to be reported (RECENT_TERMINATION_WINDOW).
	DefaultRecentTerminationWindow = time.Hour

	// DefaultTerminatingTimeout is how long a pod or namespace may remain in
	// the process of being deleted before it is reported (TERMINATING_TIMEOUT).
	DefaultTerminatingTimeout = 10 * time.Minute

	// DefaultChallengePendingTimeout is how long an ACME challenge may remain
	// pending before it is reported (CHALLENGE_PENDING_TIMEOUT).
	DefaultChallengePendingTimeout = 10 * time.Minute

	// DefaultPodRestartThreshold is how many times a container may have
	// restarted before it is reported as flapping (POD_RESTART_THRESHOLD).
	DefaultPodRestartThreshold = 5

	// DefaultVolumeReleasedTimeout is how long a PersistentVolume may remain
	// Released before it is reported (VOLUME_RELEASED_TIMEOUT).
	DefaultVolumeReleasedTimeout = 10 * time.Minute
)
