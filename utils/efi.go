package utils

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"regexp"
)

type ListMeta struct {
	v    string
	sig  int
	Size int
}

func (l *ListMeta) Extract(entry int, path string) error {
	if entry >= l.Size {
		return fmt.Errorf("index out of range")
	}

	cmd := exec.Command("efi-readvar", "-v", l.v, "-s", fmt.Sprintf("%d-%d", l.sig, entry), "-o", path)
	return cmd.Run()
}

var (
	sigPattern = regexp.MustCompile(`\s+Signature \d+`)
)

func EFIReadVar(v string) ([]*ListMeta, error) {
	//wrapper around efi-readvar
	cmd := exec.Command("efi-readvar", "-v", v)

	var buf bytes.Buffer
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	//parse buffer for date.
	pattern, err := regexp.Compile(fmt.Sprintf(`%s: List (\d+)`, v))
	if err != nil {
		return nil, err
	}

	var metas []*ListMeta
	var meta *ListMeta

	for true {
		line, err := buf.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, err
		}
		match := pattern.FindStringSubmatch(line)
		if len(match) > 0 {
			if meta != nil {
				metas = append(metas, meta)
			}

			meta = new(ListMeta)
			meta.v = v

			if _, err := fmt.Sscanf(match[1], "%d", &meta.sig); err != nil {
				return nil, err
			}

			continue
		}

		if sigPattern.FindString(line) != "" {
			meta.Size += 1
		}

		if err == io.EOF {
			break
		}
	}

	if meta != nil {
		metas = append(metas, meta)
	}

	return metas, nil
}
