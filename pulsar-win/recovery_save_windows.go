//go:build windows

package main

import (
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	comdlg32          = windows.NewLazySystemDLL("comdlg32.dll")
	pGetSaveFileNameW = comdlg32.NewProc("GetSaveFileNameW")
)

const (
	ofnOverwritePrompt = 0x00000002
	ofnPathMustExist   = 0x00000800
	ofnNoChangeDir     = 0x00000008
)

type openFileNameW struct {
	structSize       uint32
	owner            windows.Handle
	instance         windows.Handle
	filter           *uint16
	customFilter     *uint16
	maxCustomFilter  uint32
	filterIndex      uint32
	file             *uint16
	maxFile          uint32
	fileTitle        *uint16
	maxFileTitle     uint32
	initialDirectory *uint16
	title            *uint16
	flags            uint32
	fileOffset       uint16
	fileExtension    uint16
	defaultExtension *uint16
	customData       uintptr
	hook             uintptr
	templateName     *uint16
	reserved         uintptr
	reserved2        uint32
	flagsEx          uint32
}

func chooseWindowsRecoveryDestination(owner uintptr, locale ShellLocale) string {
	buffer := make([]uint16, 32768)
	copy(buffer, windows.StringToUTF16("pulsar-recovery.json"))
	filter := windows.StringToUTF16("JSON (*.json)\x00*.json\x00All files (*.*)\x00*.*\x00\x00")
	title := "Save Pulsar recovery file"
	if locale == ShellRussian {
		title = "Сохранить файл восстановления Pulsar"
	}
	of := openFileNameW{
		structSize: uint32(unsafe.Sizeof(openFileNameW{})), owner: windows.Handle(owner),
		filter: &filter[0], filterIndex: 1, file: &buffer[0], maxFile: uint32(len(buffer)),
		title: windows.StringToUTF16Ptr(title), flags: ofnOverwritePrompt | ofnPathMustExist | ofnNoChangeDir,
		defaultExtension: windows.StringToUTF16Ptr("json"),
	}
	result, _, _ := pGetSaveFileNameW.Call(uintptr(unsafe.Pointer(&of)))
	if result == 0 {
		return ""
	}
	path := windows.UTF16ToString(buffer)
	if filepath.Ext(path) == "" {
		path += ".json"
	}
	return path
}
