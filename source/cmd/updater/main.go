package main

import (
	"fmt"
	"layerhub.local/server/internal/updatekit"
	"os"
)

func main() {
	if err := updatekit.Run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "未完成 / Not completed:", err)
		os.Exit(1)
	}
}
