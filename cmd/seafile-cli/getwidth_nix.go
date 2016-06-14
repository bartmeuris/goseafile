// +build linux darwin
// +build !windows

package main

import (
	"syscall"
	"unsafe"
)


type winsize struct {
    Row    uint16
    Col    uint16
    Xpixel uint16
    Ypixel uint16
}

func GetConWidth() int {
	ws := &winsize{}
	retCode, _, errno := syscall.Syscall(syscall.SYS_IOCTL,
		uintptr(syscall.Stdin),
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(ws)))
	if errno != 0 {
		return -1;
	}
	if int(retCode) == -1 {
		return -1
	}
	return int(ws.Col)
}

