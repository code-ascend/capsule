package embed

import (
	_ "embed"
	"fmt"
)

//go:embed files/runtime.sh.tmpl
var RuntimeShTemplate string

//go:embed files/init.c.tmpl
var InitCTemplate string

//go:embed files/bash
var bashBinary []byte

//go:embed files/utils.tar.gz
var utilsTarGz []byte

func GetBash() ([]byte, error) {
	if len(bashBinary) == 0 {
		return nil, fmt.Errorf("bash binary not embedded")
	}
	return bashBinary, nil
}

func GetUtils() ([]byte, error) {
	if len(utilsTarGz) == 0 {
		return nil, fmt.Errorf("utils tarball not embedded")
	}
	return utilsTarGz, nil
}
