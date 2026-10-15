//go:build !windows && !darwin

package proxyd

import "errors"

func Detect() (Info, error) { return Info{}, errors.New("当前平台不支持") }
