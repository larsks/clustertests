package clustertests

import "time"

// Defaults for the durations that can be overridden through the environment;
// keep them in sync with the table in README.md.
const (
	// defaultUnschedulablePodTimeout is how long a pod may be unschedulable
	// before it is reported (UNSCHEDULABLE_POD_TIMEOUT).
	defaultUnschedulablePodTimeout = 10 * time.Minute

	// defaultRecentTerminationWindow is how recently a container must have
	// been OOMKilled or restarted to be reported (RECENT_TERMINATION_WINDOW).
	defaultRecentTerminationWindow = time.Hour

	// defaultTerminatingTimeout is how long a pod or namespace may remain in
	// the process of being deleted before it is reported (TERMINATING_TIMEOUT).
	defaultTerminatingTimeout = 10 * time.Minute
)
