package nativeconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type processPathLock struct {
	mutex sync.Mutex
	refs  int
}

var processLocks = struct {
	sync.Mutex
	paths map[string]*processPathLock
}{paths: make(map[string]*processPathLock)}

func acquireProcessPathLock(path string) func() {
	processLocks.Lock()
	lock := processLocks.paths[path]
	if lock == nil {
		lock = &processPathLock{}
		processLocks.paths[path] = lock
	}
	lock.refs++
	processLocks.Unlock()

	lock.mutex.Lock()
	return func() {
		lock.mutex.Unlock()
		processLocks.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(processLocks.paths, path)
		}
		processLocks.Unlock()
	}
}

func (kernel Kernel) acquireWriteLock(path string) (func() error, error) {
	releaseProcess := acquireProcessPathLock(path)
	if _, isOSFiles := kernel.files.(osFiles); !isOSFiles {
		return func() error { releaseProcess(); return nil }, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		releaseProcess()
		return nil, fmt.Errorf("create native config parent for lock: %w", err)
	}
	releaseFile, err := lockNativeConfig(path + ".agentplugins.lock")
	if err != nil {
		releaseProcess()
		return nil, fmt.Errorf("lock native config: %w", err)
	}
	return func() error {
		err := releaseFile()
		releaseProcess()
		return err
	}, nil
}

func joinUnlock(errp *error, release func() error) {
	if err := release(); err != nil {
		// The handle is always closed after the unlock attempt. Do not turn a
		// verified successful mutation into a failed lifecycle operation merely
		// because the explicit unlock syscall reported an error.
		if *errp != nil {
			*errp = errors.Join(*errp, fmt.Errorf("unlock native config: %w", err))
		}
	}
}
