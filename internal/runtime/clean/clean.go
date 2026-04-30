package clean

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"capsule/internal/runtime/mount"
)

func Run(base string) error {
	st, err := os.Stat(base)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if !st.IsDir() {
		return fmt.Errorf("not a directory: %s", base)
	}
	_ = mount.Unmount(base + "/merged")
	return os.RemoveAll(base)
}
