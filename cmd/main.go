package main

import (
	"fmt"
	"strconv"
	"time"
)

func main() {
	now := time.Now().UnixMilli()
	fmt.Printf("Time is %d\n", now)
	fmt.Printf("Time is %s", strconv.FormatInt(now, 10))
}
