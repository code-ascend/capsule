package fsutil

import "os"

func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func IsDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func IsExecutable(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode()&0111 != 0
}
