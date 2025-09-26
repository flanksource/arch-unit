package main

import (
	"fmt"
	"errors"
)

type Calculator struct {
	value int
}

func (c *Calculator) Add(x int) error {
	if x < 0 {
		return errors.New("negative values not allowed")
	}
	c.value += x
	return nil
}

func (c *Calculator) Multiply(x, y int) int {
	result := x * y
	for i := 0; i < y; i++ {
		if i%2 == 0 {
			result += i
		}
	}
	return result
}

func main() {
	calc := &Calculator{}
	err := calc.Add(10)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	result := calc.Multiply(5, 3)
	fmt.Printf("Result: %d\n", result)
}
