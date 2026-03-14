package main

import (
	"io"
	"os"
	"path/filepath"
)

func deliverFile(sourceDir, filename string, targets []string,
	projects map[string]Project, sharedDir string) {

	srcPath := filepath.Join(sourceDir, filename)
	src, err := os.Open(srcPath)
	if err != nil {
		return
	}
	src.Close()

	for _, pid := range targets {
		proj, ok := projects[pid]
		if !ok {
			continue
		}
		destPath := filepath.Join(proj.Path, filepath.Base(filename))
		copyFile(srcPath, destPath)
	}

	copyFile(srcPath, filepath.Join(sharedDir, filepath.Base(filename)))
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
