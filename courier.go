package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

func deliverFile(sourceDir, filename string, targets []string,
	projects map[string]Project, sharedDir string) {

	// Validate source path stays within sourceDir
	srcAbs, _ := filepath.Abs(filepath.Join(sourceDir, filepath.Clean(filename)))
	baseAbs, _ := filepath.Abs(sourceDir)
	if !strings.HasPrefix(srcAbs, baseAbs+string(filepath.Separator)) && srcAbs != baseAbs {
		return
	}

	src, err := os.Open(srcAbs)
	if err != nil {
		return
	}
	src.Close()

	baseName := filepath.Base(filename)
	for _, pid := range targets {
		proj, ok := projects[pid]
		if !ok {
			continue
		}
		destPath := filepath.Join(proj.Path, baseName)
		// Validate destination stays within project dir
		destAbs, _ := filepath.Abs(destPath)
		projAbs, _ := filepath.Abs(proj.Path)
		if !strings.HasPrefix(destAbs, projAbs+string(filepath.Separator)) && destAbs != projAbs {
			continue
		}
		copyFile(srcAbs, destPath)
	}

	copyFile(srcAbs, filepath.Join(sharedDir, baseName))
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
