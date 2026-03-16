//go:build linux

package server

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// clkTck is the kernel clock tick rate. It is 100 on
// virtually all Linux configurations (x86, arm64).
const clkTck = 100

// processStartTime returns the wall-clock start time of the
// process with the given PID by reading /proc/<pid>/stat.
// Field 22 (1-indexed) is starttime in clock ticks since boot.
func processStartTime(pid int) (time.Time, error) {
	data, err := os.ReadFile(
		fmt.Sprintf("/proc/%d/stat", pid),
	)
	if err != nil {
		return time.Time{}, err
	}

	// The comm field (field 2) is in parentheses and may
	// contain spaces or parens, so find the last ')' to
	// skip it reliably.
	s := string(data)
	idx := strings.LastIndex(s, ") ")
	if idx < 0 {
		return time.Time{}, fmt.Errorf(
			"malformed /proc/%d/stat", pid,
		)
	}
	// Fields after comm start at field 3 (state).
	// starttime is field 22 = index 19 after comm.
	fields := strings.Fields(s[idx+2:])
	if len(fields) < 20 {
		return time.Time{}, fmt.Errorf(
			"too few fields in /proc/%d/stat", pid,
		)
	}

	var startTicks int64
	if _, err := fmt.Sscanf(
		fields[19], "%d", &startTicks,
	); err != nil {
		return time.Time{}, fmt.Errorf(
			"parsing starttime: %w", err,
		)
	}

	bootTime, err := systemBootTime()
	if err != nil {
		return time.Time{}, err
	}
	startSec := startTicks / clkTck
	startNsec := (startTicks % clkTck) *
		(int64(time.Second) / clkTck)
	return bootTime.Add(
		time.Duration(startSec)*time.Second +
			time.Duration(startNsec),
	), nil
}
