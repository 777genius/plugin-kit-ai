package nativeconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
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

func (kernel Kernel) acquireWriteLock(path string, codec Codec) (func() error, error) {
	releaseProcess := acquireProcessPathLock(path)
	if _, isOSFiles := kernel.files.(osFiles); !isOSFiles {
		return func() error { releaseProcess(); return nil }, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		releaseProcess()
		return nil, fmt.Errorf("create native config parent for lock: %w", err)
	}
	var releaseFile func() error
	var err error
	if codec == CodecCline {
		releaseFile, err = lockClineConfig(path + ".lock")
	} else {
		releaseFile, err = lockNativeConfig(path + ".agentplugins.lock")
	}
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

func lockClineConfig(path string) (func() error, error) {
	deadline := time.Now().Add(10 * time.Second)
	for {
		err := os.Mkdir(path, 0o700)
		if err == nil {
			break
		}
		if !os.IsExist(err) {
			return nil, err
		}
		if !time.Now().Before(deadline) {
			return nil, fmt.Errorf("timed out waiting for Cline native config lock")
		}
		time.Sleep(25 * time.Millisecond)
	}
	created, err := os.Lstat(path)
	if err != nil || !created.IsDir() || created.Mode()&os.ModeSymlink != 0 {
		if err == nil {
			err = fmt.Errorf("Cline native config lock is not an exact directory")
		}
		return nil, err
	}
	return func() error {
		current, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !current.IsDir() || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(created, current) {
			return fmt.Errorf("Cline native config lock identity changed: %w", ErrConcurrentChange)
		}
		return os.Remove(path)
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
