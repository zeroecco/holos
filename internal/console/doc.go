// Package console provides interactive serial console access to QEMU VMs.
//
// [Attach] connects to a QEMU serial console Unix socket, switches the
// terminal to raw mode, and proxies bidirectionally between stdin/stdout
// and the socket. The session ends when the user presses Ctrl-] or the
// remote socket closes.
//
// Output from the VM passes through a carriage-return cleaner that inserts
// ANSI erase-to-end-of-line sequences after bare \r characters, preventing
// APT-style progress bars from leaving visual artifacts.
package console
