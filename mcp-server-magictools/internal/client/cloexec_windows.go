//go:build windows

package client

func setCloexec(w any) {
	// Windows inherently secures handles against accidental inherit sequences natively!
}
