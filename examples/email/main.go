//go:generate protoc --go_out=./pkg/proto/gen/go --go_opt=paths=source_relative ./pkg/proto/email.proto
package main

import (
	cmd "github.com/atticplaygroup/pkv/examples/email/cmd"
)

func main() {
	cmd.Execute()
}
