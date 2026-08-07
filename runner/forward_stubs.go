//go:build !darwin

package main

import "errors"

// Stubs keep forward.go's darwin dispatch branch compiling on other platforms.
// They are never reached because forward.go only calls them for runtime.GOOS
// "darwin".

func userlandForwardList() (string, error) {
	return "", errors.New("当前系统不支持用户态转发器")
}

func userlandForwardAdd(map[string]string) (string, error) {
	return "", errors.New("当前系统不支持用户态转发器")
}

func userlandForwardRemove(map[string]string) (string, error) {
	return "", errors.New("当前系统不支持用户态转发器")
}

func serveForwardCommand([]string) int {
	return 2
}
