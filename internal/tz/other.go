//go:build !windows && !darwin

package tz

import "errors"

func current() (string, error)            { return "", errors.New("当前平台不支持") }
func validate(string) error               { return errors.New("当前平台不支持") }
func set(string) error                    { return errors.New("当前平台不支持") }
func verify(string) (string, bool, error) { return "", false, errors.New("当前平台不支持") }
func needElevation() bool                 { return false }
func sameZone(a, b string) bool           { return a == b }
