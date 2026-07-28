package reviewtransaction

import "errors"

var (
	// refusal:by-design world-action: the exit is restoring private ownership/mode or removing a link/reparse point; a command must not rewrite a path it refuses to trust
	errUnsafePrivateDirectory = errors.New("unsafe private directory")
	// refusal:by-design world-action: the exit is stopping the concurrent filesystem replacement and retrying against a stable directory, not another command
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
		return nil, errors.New("private directory file limit is invalid") // refusal:by-design world-action: limits are an in-process caller invariant; the exit is a code fix, not a command
	}
	return readPrivateDirectoryFiles(path, perFileLimit, maxEntries, aggregateLimit, afterOpen)
}
