package converge

import (
	"os"
	"time"
)

type ActionType = uint8

const (
	ActionTypeCopy   ActionType = 0
	ActionTypeUpdate ActionType = 1
	ActionTypeMkdir  ActionType = 2
)

type Action struct {
	Type    ActionType
	Path    string
	Mode    os.FileMode
	ModTime time.Time
	Size    int64

	done chan struct{}
}

func NewAction(actionType ActionType, path string, mode os.FileMode, modTime time.Time, size int64) *Action {
	return &Action{
		Type:    actionType,
		Path:    path,
		Mode:    mode,
		ModTime: modTime,
		Size:    size,
		done:    make(chan struct{}),
	}
}

func (a *Action) Wait() {
	<-a.done
}

func (a *Action) Done() {
	close(a.done)
}
