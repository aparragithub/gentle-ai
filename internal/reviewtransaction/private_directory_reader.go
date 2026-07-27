package reviewtransaction

import "errors"

var (
	errUnsafePrivateDirectory   = errors.New("unsafe private directory")
	errPrivateDirectoryReplaced = errors.New("private directory changed during access")
)

type PrivateDirectoryFile struct {
	Name    string
	Payload []byte
}

// ReadPrivateDirectoryFiles anchors traversal to one opened private directory.
// Children are opened relative to that handle so pathname replacement cannot
// redirect a read into another directory.
func ReadPrivateDirectoryFiles(path string, perFileLimit int64, maxEntries int, aggregateLimit int64, afterOpen func()) ([]PrivateDirectoryFile, error) {
	if perFileLimit < 1 || maxEntries < 1 || aggregateLimit < 1 {
		return nil, errors.New("private directory file limit is invalid")
	}
	return readPrivateDirectoryFiles(path, perFileLimit, maxEntries, aggregateLimit, afterOpen)
}
