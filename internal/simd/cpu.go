package simd

import "sync"

// CPUFeatures represents the SIMD and vector instruction capabilities of the host CPU.
type CPUFeatures struct {
	HasAVX2 bool
	HasBMI2 bool
	HasNEON bool
}

var (
	mu             sync.RWMutex
	activeFeatures = detectFeatures()
)

// Features returns the currently active CPU capability flags.
func Features() CPUFeatures {
	mu.RLock()
	defer mu.RUnlock()
	return activeFeatures
}

// HasAVX2 reports whether AVX2 vector instructions are supported and enabled.
func HasAVX2() bool {
	mu.RLock()
	defer mu.RUnlock()
	return activeFeatures.HasAVX2
}

// HasBMI2 reports whether BMI2 bit-manipulation instructions are supported.
func HasBMI2() bool {
	mu.RLock()
	defer mu.RUnlock()
	return activeFeatures.HasBMI2
}

// HasNEON reports whether ARM64 NEON/ASIMD vector instructions are supported.
func HasNEON() bool {
	mu.RLock()
	defer mu.RUnlock()
	return activeFeatures.HasNEON
}

// SetCPUFeaturesForTest overrides the detected CPU capabilities for testing.
// It returns a restore function that resets capabilities to their previous state.
func SetCPUFeaturesForTest(f CPUFeatures) func() {
	mu.Lock()
	prev := activeFeatures
	activeFeatures = f
	mu.Unlock()
	return func() {
		mu.Lock()
		activeFeatures = prev
		mu.Unlock()
	}
}
