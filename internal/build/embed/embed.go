package embed

import (
	_ "embed"
	"fmt"
)

//go:generate ../../../scripts/build-runtime.sh

//go:embed files/capsule-runtime
var runtimeBinary []byte

func GetRuntime() ([]byte, error) {
	if len(runtimeBinary) == 0 {
		return nil, fmt.Errorf("capsule-runtime binary not embedded — run `go generate ./...` first")
	}
	return runtimeBinary, nil
}
