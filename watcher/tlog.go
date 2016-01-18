package watcher

import (
	"bufio"
	"fmt"
	"io"
	"time"
)

type TLog struct {
	Hash  string
	Path  string
	Epoch time.Time
}

func (t *TLog) String() string {
	return fmt.Sprintf("%s : %s | %s\n", t.Epoch.Format(time.RFC822), t.Path, t.Hash)
}

func (t *TLog) Write(w io.Writer) error {
	_, err := bufio.NewWriter(w).WriteString(t.String())
	return err
}
