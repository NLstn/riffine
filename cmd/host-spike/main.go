package main

import (
	"fmt"
	"github.com/NLstn/riffine/spikes/hosts"
	"io"
	"os"
)

func main() {
	b, err := io.ReadAll(io.LimitReader(os.Stdin, hosts.MaxFixtureBytes+1))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	result := hosts.Run(string(b))
	fmt.Println(result)
}
