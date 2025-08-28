package main

import "fmt"

func ProcessData() {
	// This should violate - fmt.Println not allowed in service files
	fmt.Println("Processing data...")
}
