package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func main() {
	data, err := os.ReadFile("../.github/workflows/release.yml")
	if err != nil {
		fmt.Println("Read error:", err)
		os.Exit(1)
	}
	var v interface{}
	err = yaml.Unmarshal(data, &v)
	if err != nil {
		fmt.Println("YAML ERROR:", err)
		os.Exit(1)
	}
	fmt.Println("YAML valid")
}
