package filetx

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

const (
	dirMode   = 0o700
	fileMode  = 0o600
	lockWait  = 5 * time.Second
	retryWait = 10 * time.Millisecond
)

func Exclusive(ctx context.Context, path string, action func() error) (err error) {
	if makeErr := os.MkdirAll(filepath.Dir(path), dirMode); makeErr != nil {
		return makeErr
	}
	guard := flock.New(path+".lock", flock.SetPermissions(fileMode))
	ctx, cancel := context.WithTimeout(ctx, lockWait)
	defer cancel()
	_, lockErr := guard.TryLockContext(ctx, retryWait)
	if lockErr != nil {
		return lockErr
	}
	defer func() {
		unlockErr := guard.Unlock()
		if err == nil {
			err = unlockErr
		}
	}()
	return action()
}

func Read(path string, limit int64) ([]byte, error) {
	if limit < 1 {
		return nil, fmt.Errorf("read %s: invalid byte limit %d", path, limit)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return read(path, file, limit)
}

func read(path string, file io.ReadCloser, limit int64) ([]byte, error) {
	reader := io.Reader(file)
	if limit < math.MaxInt64 {
		reader = io.LimitReader(file, limit+1)
	}
	body, readErr := io.ReadAll(reader)
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%s is larger than %d bytes", path, limit)
	}
	return body, nil
}
