//go:build !windows

package main

func newDefaultCredentialRepository(string) (*ProtectedCredentialRepository, error) {
	return nil, errCredentialStorageUnavailable
}
