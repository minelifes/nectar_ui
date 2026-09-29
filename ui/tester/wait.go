package tester

import "time"

// PumpUntil pumps frames until cond returns true or timeout passes (real
// time), for work that finishes on other goroutines, like loading images.
// It reports whether cond became true.
func (t *Tester) PumpUntil(cond func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		t.Pump()
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(2 * time.Millisecond)
	}
}

