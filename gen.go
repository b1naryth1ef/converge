package converge

import (
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/b1naryth1ef/converge/transport"
)

type ActionGenerator struct {
	actions   chan *Action
	tp        transport.Transport
	localPath string
}

func NewActionGenerator(tp transport.Transport, localPath string) *ActionGenerator {
	return &ActionGenerator{
		actions:   make(chan *Action, 64),
		tp:        tp,
		localPath: localPath,
	}
}

func (a *ActionGenerator) Actions() <-chan *Action {
	return a.actions
}

func (a *ActionGenerator) action(actionType ActionType, path string, mode os.FileMode, modTime time.Time, size int64) *Action {
	action := NewAction(actionType, path, mode, modTime, size)
	a.actions <- action
	return action
}

func (a *ActionGenerator) Generate(path string) error {
	sourceItems, err := a.tp.List(path)
	if err != nil {
		return err
	}

	destItems, err := os.ReadDir(filepath.Join(a.localPath, path))
	if err != nil {
		return err
	}

	destItemsMap := make(map[string]fs.DirEntry)
	for _, item := range destItems {
		destItemsMap[item.Name()] = item
	}

	var wg sync.WaitGroup
	var dependOn *Action

	for _, srcItem := range sourceItems {
		destItem := destItemsMap[srcItem.Name]

		// the remote item does not exist so we must make it via mkdir or copy
		if destItem == nil {
			if srcItem.IsDir {
				dependOn = a.action(ActionTypeMkdir, filepath.Join(path, srcItem.Name), srcItem.Mode, srcItem.ModTime, srcItem.Size)
			} else {
				a.action(ActionTypeCopy, filepath.Join(path, srcItem.Name), srcItem.Mode, srcItem.ModTime, srcItem.Size)
			}
		} else {
			destInfo, err := destItem.Info()
			if err != nil {
				return err
			}

			if srcItem.IsDir != destItem.IsDir() {
				// TODO: optional force sync instead of playing it safe
				log.Printf("WARNING: skipping path %s srcItem.IsDir != destItem.IsDir (%v / %v)", filepath.Join(path, srcItem.Name), srcItem.IsDir, destItem.IsDir())
				continue
			}

			hasChanged := (!destInfo.ModTime().Equal(srcItem.ModTime) || destInfo.Mode() != srcItem.Mode)
			if !srcItem.IsDir {
				if destInfo.Size() != srcItem.Size || hasChanged {
					a.action(ActionTypeCopy, filepath.Join(path, srcItem.Name), srcItem.Mode, srcItem.ModTime, srcItem.Size)
				}
			} else if hasChanged {
				a.action(ActionTypeUpdate, filepath.Join(path, srcItem.Name), srcItem.Mode, srcItem.ModTime, srcItem.Size)
			}
		}

		if srcItem.IsDir {
			wg.Add(1)
			go func(path string, dep *Action) {
				defer wg.Done()
				if dep != nil {
					dep.Wait()
				}
				err := a.Generate(path)
				if err != nil {
					log.Printf("generate sub-error: %v", err)
				}
			}(filepath.Join(path, srcItem.Name), dependOn)
		}
	}

	wg.Wait()
	return nil
}
