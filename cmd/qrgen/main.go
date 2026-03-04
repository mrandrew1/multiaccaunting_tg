package main

import (
	"fmt"
	"os"

	"github.com/skip2/go-qrcode"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/qrgen <url>")
		os.Exit(1)
	}
	url := os.Args[1]
	out := "qr.png"
	if err := qrcode.WriteFile(url, qrcode.Medium, 256, out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("QR saved to", out)
}
