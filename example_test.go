package simdnorm_test

import (
	"fmt"

	"simdnorm"
)

func ExampleNormalize() {
	v := []float32{3, 4}
	norm := simdnorm.Normalize(v)
	fmt.Println(norm, v)
	// Output: 5 [0.6 0.8]
}
