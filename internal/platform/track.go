package platform

import "sync"

var (
	trackMu          sync.Mutex
	presenceCounts   = map[string]int{}
	windowCounts     = map[string]int{}
)

func podKey(ns, name string) string { return ns + "/" + name }

func IncPresence(ns, name string) {
	trackMu.Lock()
	presenceCounts[podKey(ns, name)]++
	trackMu.Unlock()
}

func DecPresence(ns, name string) {
	trackMu.Lock()
	if presenceCounts[podKey(ns, name)] > 0 {
		presenceCounts[podKey(ns, name)]--
	}
	trackMu.Unlock()
}

func IncWindow(ns, name string) {
	trackMu.Lock()
	windowCounts[podKey(ns, name)]++
	trackMu.Unlock()
}

func CountPresence(names []string, ns string) int {
	trackMu.Lock()
	defer trackMu.Unlock()
	n := 0
	for _, name := range names {
		n += presenceCounts[podKey(ns, name)]
	}
	return n
}

func CountWindows(names []string, ns string) int {
	trackMu.Lock()
	defer trackMu.Unlock()
	n := 0
	for _, name := range names {
		n += windowCounts[podKey(ns, name)]
	}
	return n
}
