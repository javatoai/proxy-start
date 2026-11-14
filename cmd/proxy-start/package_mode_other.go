//go:build !windows

package main

func runPackagedMode([]string) (bool, error) { return false, nil }
