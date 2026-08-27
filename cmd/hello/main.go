// Command hello is the Phase 0 toolchain sanity binary (spec 18.2 /
// 18.1's cmd/hello): if this prints and `go test ./...` is green, the
// toolchain on this machine is ready to build the rest of ShadowForge L1.
package main

import "fmt"

func main() {
	fmt.Println("ShadowForge L1 toolchain OK — hello, shadow.")
}
