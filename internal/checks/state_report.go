package checks

import (
	"fmt"

	"github.com/hashicorp/terraform/internal/addrs"
)

// These are the "Report"-prefixed methods of Checks used by Terraform Core
// to gradually signal the results of checks during a plan or apply operation.

// ReportCheckableObjects is the interface by which Terraform Core should
// tell the State object which specific checkable objects were declared
// by the given configuration object.
//
// This method will panic if the given configuration address isn't one known
// by this Checks to have pending checks, and if any of the given object
// addresses don't belong to the given configuration address.
func (c *State) ReportCheckableObjects(configAddr addrs.ConfigCheckable, objectAddrs addrs.Set[addrs.Checkable]) {
	c.mu.Lock()
	defer c.mu.Unlock()

	st, ok := c.statuses.GetOk(configAddr)
	if !ok {
		panic(fmt.Sprintf("checkable objects report for unknown configuration object %s", configAddr))
	}
	if st.objects.Elems != nil {
		// Can only report checkable objects once per configuration object
		panic(fmt.Sprintf("duplicate checkable objects report for %s ", configAddr))
	}

	// At this point we pre-populate all of the check results as StatusUnknown,
	// so that even if we never hear from Terraform Core again we'll still
	// remember that these results were all pending.
	st.objects = addrs.MakeMap[addrs.Checkable, map[addrs.CheckType][]Status]()
	for _, objectAddr := range objectAddrs {
		if gotConfigAddr := objectAddr.ConfigCheckable(); !addrs.Equivalent(configAddr, gotConfigAddr) {
			// All of the given object addresses must belong to the specified configuration address
			panic(fmt.Sprintf("%s belongs to %s, not %s", objectAddr, gotConfigAddr, configAddr))
		}

		checks := make(map[addrs.CheckType][]Status, len(st.checkTypes))
		for checkType, count := range st.checkTypes {
			// NOTE: This is intentionally a slice of count of the zero value
			// of Status, which is StatusUnknown to represent that we don't
			// yet have a report for that particular check.
			checks[checkType] = make([]Status, count)
		}

		st.objects.Put(objectAddr, checks)
	}
}

// ReportCheckResult is the interface by which Terraform Core should tell the
// State object the result of a specific check for an object that was
// previously registered with ReportCheckableObjects.
//
// If the given object address doesn't match a previously-reported object,
// or if the check index is out of bounds for the number of checks expected
// of the given type, this method will panic to indicate a bug in the caller.
//
// This method will also panic if the specified check already had a known
// status; each check should have its result reported only once.
func (c *State) ReportCheckResult(objectAddr addrs.Checkable, checkType addrs.CheckType, index int, status Status) {
	c.mu.Lock()
	defer c.mu.Unlock()

	configAddr := objectAddr.ConfigCheckable()

	st, ok := c.statuses.GetOk(configAddr)
	if !ok {
		panic(fmt.Sprintf("checkable object status report for unknown configuration object %s", configAddr))
	}

	checks, ok := st.objects.GetOk(objectAddr)
	if !ok {
		panic(fmt.Sprintf("checkable object status report for unexpected checkable object %s", objectAddr))
	}

	if index >= len(checks[checkType]) {
		panic(fmt.Sprintf("%s index %d out of range for %s", checkType, index, objectAddr))
	}
	if checks[checkType][index] != StatusUnknown {
		panic(fmt.Sprintf("duplicate status report for %s %s %d", objectAddr, checkType, index))
	}

	checks[checkType][index] = status
}