package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}

type gitFacts struct {
	Commit string
	Branch string
	Status string
	Err    string
}

func readGit(root string) gitFacts {
	var g gitFacts
	commit, err := gitOut(root, "rev-parse", "HEAD")
	if err != nil {
		g.Err = err.Error()
		return g
	}
	g.Commit = commit
	branch, err := gitOut(root, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		g.Err = err.Error()
		return g
	}
	g.Branch = branch
	status, err := gitOut(root, "status", "--porcelain")
	if err != nil {
		g.Err = err.Error()
		return g
	}
	g.Status = status
	return g
}

func gitOut(root string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
