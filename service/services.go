package service

// DiffResult explains broadly explains the difference between the remote and local versions of a file
type DiffResult uint8

const (
	// DiffResultMatch means both files exist and have the same content
	DiffResultMatch = DiffResult(iota)

	// DiffResultMismatch mean both files exist, but their content does not match
	DiffResultMismatch

	// DiffResultOnlyExistsRemote means the file only exists on the remote machine
	DiffResultOnlyExistsRemote

	// DiffResultOnlyExistsLocal means that the file only exists on the local machine
	DiffResultOnlyExistsLocal
)

// WalkDiffsCallback is the callback function type in StorageService.WalkDifs
type WalkDiffsCallback func(string, DiffResult) error

// StorageService is a application that allows users to store files remotely (e.g. Dropbox, Google Drive, Amazon S3)
type StorageService interface {
	// WalkDiffs walks through the differences between a local and remote directory, recursively
	WalkDiffs(local, remote string, callback WalkDiffsCallback) error

	// Upload uploads a local file, "local" to a remote file "remote"
	Upload(local, remote string) error

	// Download downloads a remote file "remote" to a local file "local"
	Download(local, remote string) error

	// Delete removes a file from the storage service
	Delete(remote string) error
}
