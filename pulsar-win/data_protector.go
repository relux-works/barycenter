package main

import "errors"

const (
	cryptprotectUIForbidden  uint32 = 0x1
	cryptprotectLocalMachine uint32 = 0x4
)

type protectedAllocation interface {
	Bytes() []byte
	Free() error
}

type nativeDataProtectionAPI interface {
	Protect([]byte, uint32) (protectedAllocation, error)
	Unprotect([]byte, uint32) (protectedAllocation, error)
}

type dataProtector interface {
	Protect([]byte) ([]byte, error)
	Unprotect([]byte) ([]byte, error)
}

type dpapiDataProtector struct{ api nativeDataProtectionAPI }

func (p dpapiDataProtector) Protect(plaintext []byte) ([]byte, error) {
	allocation, callErr := p.api.Protect(plaintext, cryptprotectUIForbidden)
	return copyAndFreeProtectedAllocation(allocation, callErr, errProtectData)
}

func (p dpapiDataProtector) Unprotect(ciphertext []byte) ([]byte, error) {
	allocation, callErr := p.api.Unprotect(ciphertext, cryptprotectUIForbidden)
	return copyAndFreeProtectedAllocation(allocation, callErr, errUnprotectData)
}

func copyAndFreeProtectedAllocation(allocation protectedAllocation, callErr error, stable error) ([]byte, error) {
	if allocation == nil {
		if callErr != nil {
			return nil, stable
		}
		return nil, stable
	}
	var copied []byte
	native := allocation.Bytes()
	if callErr == nil {
		copied = append([]byte(nil), native...)
	}
	zeroBytes(native)
	if freeErr := allocation.Free(); freeErr != nil {
		zeroBytes(copied)
		return nil, stable
	}
	if callErr != nil || len(copied) == 0 {
		zeroBytes(copied)
		return nil, stable
	}
	return copied, nil
}

var (
	errProtectData   = errors.New("protect credential data failed")
	errUnprotectData = errors.New("unprotect credential data failed")
)
