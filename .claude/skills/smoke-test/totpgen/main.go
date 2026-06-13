// Generates a current TOTP code for a base32 secret, using the same library the
// bastion uses. Run: go run ./.claude/skills/smoke-test/totpgen <secret>
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/pquerna/otp/totp"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: totpgen <base32-secret>")
		os.Exit(2)
	}
	code, err := totp.GenerateCode(os.Args[1], time.Now())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(code)
}
