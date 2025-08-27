package main

import "fmt"

func main() {
	// This should be allowed because of [main.go] override
	fmt.Println("Hello from main!")
}