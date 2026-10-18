//go:build !windows

package main

import "fmt"

func main() { fmt.Println("proxy-start-helper is only supported on Windows") }
